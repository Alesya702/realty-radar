package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// TestRuleCodesCoverAllConstants keeps RuleCodes exhaustive: adapters rely on
// it to check that every code has a user message, so a code that is declared
// but not listed would leave that check false positive (review L2).
func TestRuleCodesCoverAllConstants(t *testing.T) {
	listed := make(map[string]bool, len(RuleCodes))
	for _, code := range RuleCodes {
		if listed[code] {
			t.Errorf("rule code %q is listed twice in RuleCodes", code)
		}
		listed[code] = true
	}

	declared := declaredRuleCodes(t)
	if len(declared) == 0 {
		t.Fatal("no Code* constants found in errors.go")
	}
	for name, value := range declared {
		if !listed[value] {
			t.Errorf("constant %s = %q is not in RuleCodes", name, value)
		}
		delete(listed, value)
	}
	for code := range listed {
		t.Errorf("RuleCodes contains %q, which is not declared as a constant", code)
	}
}

// declaredRuleCodes returns the Code* constants of errors.go by name and value.
func declaredRuleCodes(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "errors.go", nil, 0)
	if err != nil {
		t.Fatalf("parse errors.go: %v", err)
	}
	codes := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if !strings.HasPrefix(name.Name, "Code") || i >= len(value.Values) {
					continue
				}
				lit, ok := value.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("constant %s: %v", name.Name, err)
				}
				codes[name.Name] = unquoted
			}
		}
	}
	return codes
}
