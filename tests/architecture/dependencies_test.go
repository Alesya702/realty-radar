// Package architecture_test enforces dependency rules between layers and
// modules (docs/architecture/modules.md, docs/decisions/0004-module-boundaries.md).
package architecture_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	goModule       = "realtyradar"
	srcPath        = goModule + "/src/"
	modulesPath    = srcPath + "modules/"
	platformPath   = srcPath + "platform"
	webPath        = srcPath + "web"
	appPath        = srcPath + "app"
	srcDirFromHere = "../../src"
)

// allowedModuleDeps lists which business modules each module may import
// (always through the root package only). Policy (ADR-0004):
//   - core depends on no other business module;
//   - by default other modules may depend only on core's public contract;
//   - a direct dependency between non-core modules is allowed only for a real
//     business need, must be approved by its own ADR and must be listed here
//     with a comment citing that ADR.
//
// Any change to this map is an architectural decision and requires an ADR.
var allowedModuleDeps = map[string][]string{
	"core":           {},
	"offeranalytics": {"core"},
	"collection":     {"core"},
	"declarations":   {"core"},
	"floorplans":     {"core"},
	"benchmark":      {"core"},
	"location":       {"core"},
}

// forbiddenInDomain are imports that tie domain code to infrastructure.
var forbiddenInDomain = []string{
	"database/sql",
	"net/http",
	"html/template",
	"github.com/jackc/pgx",
	platformPath,
	webPath,
}

// checkImport returns an error if package `from` must not import `imp`.
func checkImport(from, imp string) error {
	if inArea(from, platformPath) {
		if strings.HasPrefix(imp, modulesPath) || inArea(imp, webPath) || inArea(imp, appPath) {
			return fmt.Errorf("platform must not depend on modules, web or app")
		}
	}

	if inArea(from, webPath) {
		if strings.HasPrefix(imp, modulesPath) || inArea(imp, appPath) {
			return fmt.Errorf("web must not depend on business modules or app")
		}
	}

	fromModule, fromRest, fromIsModule := splitModule(from)
	if !fromIsModule {
		return nil
	}

	allowed, known := allowedModuleDeps[fromModule]
	if !known {
		return fmt.Errorf("module %q is not registered in allowedModuleDeps", fromModule)
	}

	if inArea(imp, appPath) {
		return fmt.Errorf("modules must not depend on the composition root (app)")
	}

	if isDomain(fromRest) {
		for _, f := range forbiddenInDomain {
			if inArea(imp, f) {
				return fmt.Errorf("domain packages must not import infrastructure (%s)", f)
			}
		}
	}

	impModule, impRest, impIsModule := splitModule(imp)
	if !impIsModule {
		return nil
	}

	if impModule == fromModule {
		if isDomain(fromRest) && !isDomain(impRest) {
			return fmt.Errorf("domain must depend only on domain packages of its own module")
		}
		if inArea(fromRest, "internal/app") && inArea(impRest, "internal/adapters") {
			return fmt.Errorf("use cases (internal/app) must not depend on adapters of their module")
		}
		return nil
	}

	if !slices.Contains(allowed, impModule) {
		return fmt.Errorf("module %q must not depend on module %q (allowed: %v)", fromModule, impModule, allowed)
	}
	if impRest != "" {
		return fmt.Errorf("module %q may use module %q only through its root package, not %q", fromModule, impModule, impRest)
	}
	return nil
}

// splitModule splits ".../modules/<name>/<rest>" into name and rest.
func splitModule(pkg string) (name, rest string, ok bool) {
	after, found := strings.CutPrefix(pkg, modulesPath)
	if !found || after == "" {
		return "", "", false
	}
	name, rest, _ = strings.Cut(after, "/")
	return name, rest, true
}

func inArea(pkg, area string) bool {
	return pkg == area || strings.HasPrefix(pkg, area+"/")
}

func isDomain(rest string) bool {
	return inArea(rest, "internal/domain")
}

func TestCheckImportRules(t *testing.T) {
	m := func(p string) string { return modulesPath + p }
	tests := []struct {
		from, imp string
		wantErr   bool
	}{
		{m("collection/internal/app"), m("core"), false},
		{m("collection/internal/adapters/sources/example"), m("core"), false},
		{m("collection/internal/app"), m("core/internal/domain"), true},
		{m("collection/internal/app"), m("core/contracts"), true},
		{m("core/internal/app"), m("collection"), true},
		{m("offeranalytics/internal/app"), m("declarations"), true},
		{m("core/internal/domain"), "github.com/jackc/pgx/v5", true},
		{m("core/internal/domain"), "net/http", true},
		{m("core/internal/domain"), platformPath + "/config", true},
		{m("core/internal/domain"), m("core/internal/adapters/postgres"), true},
		{m("core/internal/domain"), m("core"), true},
		{m("core/internal/domain/lot"), m("core/internal/domain/money"), false},
		{m("core/internal/domain"), "time", false},
		{m("core/internal/app"), m("core/internal/domain"), false},
		{m("core/internal/adapters/postgres"), "github.com/jackc/pgx/v5", false},
		{m("core/internal/adapters/http"), webPath, false},
		{m("core"), m("core/internal/app"), false},
		{m("core"), webPath, false},
		{m("core/internal/app"), m("core/internal/adapters/postgres"), true},
		{m("core/internal/adapters/http"), m("core/internal/app"), false},
		{m("collection/internal/app"), m("core/internal/adapters/http"), true},
		{m("offeranalytics/internal/adapters/postgres"), m("core/internal/adapters/postgres"), true},
		{m("core/internal/domain"), "net/url", false},
		{m("core/internal/app"), appPath, true},
		{m("unregistered/internal/app"), m("core"), true},
		{platformPath + "/database", m("core"), true},
		{platformPath + "/httpserver", webPath, true},
		{webPath, m("core"), true},
		{appPath, m("core"), false},
	}
	for _, tt := range tests {
		err := checkImport(tt.from, tt.imp)
		if (err != nil) != tt.wantErr {
			t.Errorf("checkImport(%q, %q) error = %v, wantErr %v", tt.from, tt.imp, err, tt.wantErr)
		}
	}
}

func TestSourceTreeRespectsDependencyRules(t *testing.T) {
	fset := token.NewFileSet()
	files := 0

	err := filepath.WalkDir(srcDirFromHere, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		files++

		rel, err := filepath.Rel(srcDirFromHere, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := srcPath + filepath.ToSlash(rel)

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			imp, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if err := checkImport(pkg, imp); err != nil {
				t.Errorf("%s imports %q: %v", filepath.ToSlash(path), imp, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk source tree: %v", err)
	}
	if files == 0 {
		t.Fatal("no Go files found: the test is not scanning the source tree")
	}
}

func TestEveryModuleIsRegistered(t *testing.T) {
	modulesDir := filepath.Join(srcDirFromHere, "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		t.Fatalf("read modules directory: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			if _, ok := allowedModuleDeps[e.Name()]; !ok {
				t.Errorf("module directory %q is not registered in allowedModuleDeps", e.Name())
			}
		}
	}
	for name := range allowedModuleDeps {
		if _, err := os.Stat(filepath.Join(modulesDir, name)); err != nil {
			t.Errorf("allowedModuleDeps references missing module %q", name)
		}
	}
}
