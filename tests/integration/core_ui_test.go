//go:build integration

package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"realtyradar/src/modules/core"
	"realtyradar/src/web"
)

type uiClient struct {
	t   *testing.T
	mux *http.ServeMux
}

func newUI(t *testing.T) (*uiClient, *core.Module) {
	m, _ := newCore(t)
	mux := http.NewServeMux()
	web.Register(mux, web.Info{Version: "test", Env: "test", Nav: core.Nav()})
	m.RegisterRoutes(mux)
	return &uiClient{t: t, mux: mux}, m
}

func (c *uiClient) get(path string) (int, string) {
	c.t.Helper()
	rec := httptest.NewRecorder()
	c.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, _ := io.ReadAll(rec.Body)
	return rec.Code, string(body)
}

func (c *uiClient) post(path string, form url.Values) (status int, location, body string) {
	c.t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c.mux.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, rec.Header().Get("Location"), string(b)
}

func (c *uiClient) mustRedirect(path string, form url.Values) string {
	c.t.Helper()
	status, location, body := c.post(path, form)
	if status != http.StatusSeeOther {
		c.t.Fatalf("POST %s: status %d, want 303; body: %s", path, status, alertText(body))
	}
	return location
}

var alertPattern = regexp.MustCompile(`role="alert">([^<]*)<`)

func alertText(body string) string {
	if m := alertPattern.FindStringSubmatch(body); m != nil {
		return m[1]
	}
	if len(body) > 300 {
		return body[:300]
	}
	return body
}

// nbsp is the thousands separator used by the pages.
func rubText(s string) string { return strings.ReplaceAll(s, " ", " ") + " ₽" }

func TestCoreUIScenario(t *testing.T) {
	c, m := newUI(t)
	manual := manualSource(t, m)
	observedAt := time.Now().Add(-2 * time.Hour).In(time.FixedZone("MSK", 3*3600))

	// 1. Projects list and start page.
	for _, path := range []string{"/", "/projects", "/developers", "/portfolio", "/data-sources"} {
		if status, _ := c.get(path); status != http.StatusOK {
			t.Fatalf("GET %s = %d", path, status)
		}
	}

	// 2–3. Developer and project.
	devName := unique("Девелопер UI")
	c.mustRedirect("/developers", url.Values{"name": {devName}})
	_, devPage := c.get("/developers")
	if !strings.Contains(devPage, devName) {
		t.Fatal("developer is not listed")
	}
	devID := regexp.MustCompile(`<option value="([0-9a-f-]{36})"[^>]*>` + regexp.QuoteMeta(devName)).FindStringSubmatch(mustGet(t, c, "/projects"))
	if devID == nil {
		t.Fatal("developer is not offered in the project form")
	}
	projectName := unique("ЖК UI")
	projectPath := c.mustRedirect("/projects", url.Values{"name": {projectName}, "developer_id": {devID[1]}, "city": {"Москва"}})
	if !strings.HasPrefix(projectPath, "/projects/") {
		t.Fatalf("project redirect = %q", projectPath)
	}
	page := mustGet(t, c, projectPath)
	if !strings.Contains(page, projectName) || !strings.Contains(page, devName) || !strings.Contains(page, "Внешний проект") {
		t.Fatal("project card does not show the project, developer and role")
	}

	// Validation error keeps the user on the page with a message.
	if status, _, body := c.post("/projects", url.Values{"name": {"  "}}); status != http.StatusUnprocessableEntity || !strings.Contains(body, "Заполните поле «Название»") {
		t.Fatalf("empty project name: status %d, alert %q", status, alertText(body))
	}

	// 4–5. Building and section.
	c.mustRedirect(projectPath+"/buildings", url.Values{"name": {"Корпус 1"}})
	page = mustGet(t, c, projectPath)
	buildingID := regexp.MustCompile(`value="b:([0-9a-f-]{36})"`).FindStringSubmatch(page)
	if buildingID == nil {
		t.Fatal("building is not offered for lots")
	}
	c.mustRedirect(projectPath+"/sections", url.Values{"building_id": {buildingID[1]}, "name": {"2"}})
	page = mustGet(t, c, projectPath)
	sectionID := regexp.MustCompile(`value="s:([0-9a-f-]{36})"`).FindStringSubmatch(page)
	if sectionID == nil {
		t.Fatal("section is not offered for lots")
	}

	// 6. Lot with its first observation.
	lotForm := url.Values{
		"placement": {"b:" + buildingID[1]}, "lot_type": {"apartment"}, "number": {"145"}, "floor": {"7"},
		"rooms": {"2"}, "area_total": {"67,4"}, "price": {"15 900 000"}, "status": {"available"},
		"observed_at": {observedAt.Format("2006-01-02T15:04")}, "data_source_id": {manual.ID.String()},
	}
	invalid := cloneValues(lotForm)
	invalid.Set("price_before_discount", "15 000 000")
	if status, _, body := c.post(projectPath+"/lots", invalid); status != http.StatusUnprocessableEntity || !strings.Contains(body, "Цена до скидки должна быть больше цены") || !strings.Contains(body, `value="145"`) {
		t.Fatalf("invalid discount: status %d, alert %q", status, alertText(body))
	}
	lotPath := c.mustRedirect(projectPath+"/lots", lotForm)
	if !strings.HasPrefix(lotPath, "/lots/") {
		t.Fatalf("lot redirect = %q", lotPath)
	}
	lotInSection := cloneValues(lotForm)
	lotInSection.Set("placement", "s:"+sectionID[1])
	lotInSection.Set("number", "146")
	c.mustRedirect(projectPath+"/lots", lotInSection)
	if status, _, body := c.post(projectPath+"/lots", lotForm); status != http.StatusUnprocessableEntity || !strings.Contains(body, "уже есть лот") {
		t.Fatalf("duplicate lot: status %d, alert %q", status, alertText(body))
	}

	// 7, 9. Lot card with the current state.
	card := mustGet(t, c, lotPath)
	for _, want := range []string{rubText("15 900 000"), "67,4 м²", "Свободен", "Корпус 1"} {
		if !strings.Contains(card, want) {
			t.Fatalf("lot card does not contain %q", want)
		}
	}

	// 8. A new observation changes the price.
	second := observedAt.Add(time.Hour)
	c.mustRedirect(lotPath+"/observations", url.Values{
		"price": {"16 400 000"}, "price_before_discount": {""}, "area_total": {"67.4"}, "rooms": {"2"},
		"status": {"available"}, "presence": {"listed"}, "finish": {""},
		"observed_at": {second.Format("2006-01-02T15:04")}, "data_source_id": {manual.ID.String()},
	})

	// 10. Current state and history.
	card = mustGet(t, c, lotPath)
	if !strings.Contains(card, rubText("16 400 000")) || !strings.Contains(card, rubText("15 900 000")) || strings.Count(card, "<td class=\"num\">"+rubText("16 400 000")) != 1 {
		t.Fatal("history does not show both prices")
	}
	at := observedAt.Add(30 * time.Minute).Format("2006-01-02T15:04")
	if _, atPage := c.get(lotPath + "?at=" + url.QueryEscape(at)); !strings.Contains(atPage, "цена <strong>"+rubText("15 900 000")) {
		t.Fatal("state at date does not show the earlier price")
	}
	if status, _, body := c.post(lotPath+"/observations", url.Values{
		"price": {"1"}, "status": {"available"}, "presence": {"listed"},
		"observed_at": {observedAt.Add(-24 * time.Hour).Format("2006-01-02T15:04")}, "data_source_id": {manual.ID.String()},
	}); status != http.StatusUnprocessableEntity || !strings.Contains(body, "раньше текущего состояния") {
		t.Fatalf("out-of-order observation: status %d, alert %q", status, alertText(body))
	}

	// Portfolio and relations.
	c.mustRedirect(projectPath+"/portfolio", url.Values{"valid_from": {"2026-01-01"}, "note": {"наш проект"}})
	competitorPath := c.mustRedirect("/projects", url.Values{"name": {unique("ЖК Конкурент UI")}})
	competitorID := strings.TrimPrefix(competitorPath, "/projects/")
	c.mustRedirect(projectPath+"/relations", url.Values{"related_project_id": {competitorID}, "relation_type": {"direct_competitor"}, "valid_from": {"2026-01-01"}, "rationale": {"соседний участок"}})
	page = mustGet(t, c, projectPath)
	for _, want := range []string{"Проект компании (портфель)", "Прямой конкурент", "соседний участок"} {
		if !strings.Contains(page, want) {
			t.Fatalf("project card does not contain %q", want)
		}
	}
	if !strings.Contains(mustGet(t, c, "/portfolio"), projectName) {
		t.Fatal("portfolio page does not list the project")
	}
	if strings.Contains(mustGet(t, c, competitorPath), "Проект компании (портфель)") {
		t.Fatal("competitor must stay an external project")
	}

	// Data sources.
	code := "ui-" + strings.ReplaceAll(unique("src")[4:], " ", "")
	c.mustRedirect("/data-sources", url.Values{"code": {code}, "name": {"Прайс-листы (тест)"}, "kind": {"document_channel"}})
	if !strings.Contains(mustGet(t, c, "/data-sources"), code) {
		t.Fatal("data source is not listed")
	}

	// Not found.
	if status, _ := c.get("/lots/00000000-0000-7000-8000-000000000000"); status != http.StatusNotFound {
		t.Errorf("missing lot status = %d, want 404", status)
	}
}

func mustGet(t *testing.T, c *uiClient, path string) string {
	t.Helper()
	status, body := c.get(path)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, status, alertText(body))
	}
	return body
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vals := range v {
		out[k] = append([]string(nil), vals...)
	}
	return out
}
