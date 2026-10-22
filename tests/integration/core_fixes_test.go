//go:build integration

package integration_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"realtyradar/src/modules/core"
)

// TestCoreLotsWithoutNumber checks that the lot number is optional: lots
// without a number are stored with NULL, are never merged with each other by
// placement, and are identified by the source key when it is known.
func TestCoreLotsWithoutNumber(t *testing.T) {
	ctx := context.Background()
	m, pool := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	b, err := m.AddBuilding(ctx, p.ID, "Корпус 1")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Hour)
	noNumber := func(ref string, key *string) core.LotObservation {
		return core.LotObservation{Ref: ref, SourceKey: key,
			Target: core.LotTarget{NewLot: &core.NewLotPlacement{BuildingRef: "b", Type: core.LotTypeParking}},
			State:  core.ObservedState{Status: core.Value(core.LotStatusAvailable)}}
	}
	ingest := func(items ...core.LotObservation) (core.BatchResult, error) {
		return m.IngestObservationBatch(ctx, core.ObservationBatch{
			DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID,
			Buildings: []core.BuildingDecision{existingBuilding("b", b.ID)}, Items: items,
		})
	}
	countWithoutNumber := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.lots WHERE project_id = $1 AND number IS NULL`, p.ID.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	first, err := ingest(noNumber("first", nil))
	if err != nil {
		t.Fatalf("a lot without a number must be accepted: %v", err)
	}
	second, err := ingest(noNumber("second", nil))
	if err != nil {
		t.Fatalf("a second lot without a number in the same building must be accepted: %v", err)
	}
	if first.Items[0].LotID == second.Items[0].LotID || !second.Items[0].LotIsNew {
		t.Fatal("lots without a number must not be merged")
	}
	if _, err := ingest(noNumber("a", nil), noNumber("b", nil)); err != nil {
		t.Fatalf("two lots without a number in one batch are not duplicates: %v", err)
	}
	if n := countWithoutNumber(); n != 4 {
		t.Fatalf("lots stored without a number = %d, want 4 (number must be NULL)", n)
	}

	// The source key identifies the listing: a "new lot" with a known key is refused.
	key := unique("ad")
	if _, err := ingest(noNumber("keyed", &key)); err != nil {
		t.Fatal(err)
	}
	_, err = ingest(noNumber("same-key", &key))
	var be *core.BatchError
	if !errors.As(err, &be) {
		t.Fatalf("error = %v, want a batch error", err)
	}
	mustRule(t, be.Items[0].Err, "source_key_conflict")
	if n := countWithoutNumber(); n != 5 {
		t.Fatalf("a refused batch must not create a lot: %d lots without number", n)
	}

	// A lot without a number is observed by its id.
	lotID := first.Items[0].LotID
	res, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at.Add(time.Minute), ProjectID: p.ID,
		Items: []core.LotObservation{{Ref: "x", Target: core.LotTarget{ExistingLotID: &lotID}, State: core.ObservedState{Status: core.Value(core.LotStatusSold)}}},
	})
	if err != nil || res.Items[0].Change != core.ChangeChanged {
		t.Fatalf("observation of a lot without a number: %+v, %v", res, err)
	}
	lot, err := m.GetLot(ctx, lotID)
	if err != nil || lot.Placement.Number != "" {
		t.Fatalf("lot = %+v, %v; want no number", lot, err)
	}

	// An empty string is never stored as a number.
	_, err = pool.Exec(ctx, `
		INSERT INTO core.lots (id, project_id, lot_type, number, created_at, updated_at)
		VALUES ($1, $2, 'apartment', ' ', now(), now())`, "01a0a5df-0000-7000-8000-00000000abce", p.ID.String())
	assertPgCode(t, err, "23514") // check_violation
}

// TestCorePriceOnRequest checks that a lot may have no price, that "price on
// request" is kept apart from "not stated", and that the price before
// discount follows the price.
func TestCorePriceOnRequest(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	at := time.Now().Add(-3 * time.Hour)
	batch := func(at time.Time, item core.LotObservation) (core.BatchResult, error) {
		return m.IngestObservationBatch(ctx, core.ObservationBatch{
			DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID, Items: []core.LotObservation{item},
		})
	}
	newLot := func(number string, st core.ObservedState) core.LotObservation {
		return core.LotObservation{Ref: number, Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: number}}, State: st}
	}
	current := func(lotID core.ID) core.LotState {
		t.Helper()
		h, err := m.GetLotHistory(ctx, lotID)
		if err != nil || h.Current() == nil {
			t.Fatalf("history: %+v, %v", h, err)
		}
		return h.Current().State
	}

	res, err := batch(at, newLot("1", core.ObservedState{Status: core.Value(core.LotStatusAvailable)}))
	if err != nil {
		t.Fatalf("a lot without a price must be accepted: %v", err)
	}
	if st := current(res.Items[0].LotID); st.Price != nil || st.PriceOnRequest || st.PricePerSqm() != nil {
		t.Errorf("lot without price: %+v", st)
	}

	res, err = batch(at, newLot("2", core.ObservedState{Status: core.Value(core.LotStatusAvailable), PriceOnRequest: core.Value(true), AreaTotal: core.Value(core.Area(15000))}))
	if err != nil {
		t.Fatalf("a lot with price on request must be accepted: %v", err)
	}
	lotID := res.Items[0].LotID
	if st := current(lotID); st.Price != nil || !st.PriceOnRequest || st.PricePerSqm() != nil {
		t.Errorf("lot with price on request: %+v", st)
	}

	existing := func(st core.ObservedState) core.LotObservation {
		return core.LotObservation{Ref: "x", Target: core.LotTarget{ExistingLotID: &lotID}, State: st}
	}
	if _, err := batch(at.Add(time.Hour), existing(core.ObservedState{Price: core.Value(rub(90_000_000)), PriceBeforeDiscount: core.Value(rub(95_000_000))})); err != nil {
		t.Fatal(err)
	}
	if st := current(lotID); st.Price == nil || *st.Price != rub(90_000_000) || st.PriceOnRequest {
		t.Errorf("a stated price must end price on request: %+v", st)
	}
	if _, err := batch(at.Add(2*time.Hour), existing(core.ObservedState{PriceOnRequest: core.Value(true)})); err != nil {
		t.Fatal(err)
	}
	if st := current(lotID); st.Price != nil || st.PriceBeforeDiscount != nil || !st.PriceOnRequest {
		t.Errorf("price on request must clear the price and the price before discount: %+v", st)
	}

	_, err = batch(at, newLot("3", core.ObservedState{Price: core.Value(rub(1_000_000)), PriceOnRequest: core.Value(true)}))
	var be *core.BatchError
	if !errors.As(err, &be) {
		t.Fatalf("error = %v, want a batch error", err)
	}
	mustRule(t, be.Items[0].Err, "price_on_request_with_price")
}

// TestCoreIncomingRelations checks that a relation is visible from the related
// project without a mirror record.
func TestCoreIncomingRelations(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	a, b := newProject(t, m), newProject(t, m)
	if _, err := m.EstablishProjectRelation(ctx, core.RelationInput{
		SubjectProjectID: b.ID, RelatedProjectID: a.ID, Type: core.RelationDirectCompetitor, ValidFrom: time.Now().AddDate(0, -1, 0),
	}); err != nil {
		t.Fatal(err)
	}
	incoming, err := m.ListIncomingProjectRelations(ctx, a.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(incoming) != 1 || incoming[0].SubjectProject.ID != b.ID || !incoming[0].Active || incoming[0].Relation.Type != core.RelationDirectCompetitor {
		t.Fatalf("incoming relations of A = %+v, want B → A", incoming)
	}
	own, err := m.ListProjectRelations(ctx, a.ID, time.Now())
	if err != nil || len(own) != 0 {
		t.Fatalf("no mirror relation must be created: %+v, %v", own, err)
	}
}

// TestCoreUpdateProject checks editing of the project parameters.
func TestCoreUpdateProject(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	p := newProject(t, m)
	dev, err := m.CreateDeveloper(ctx, unique("Девелопер"))
	if err != nil {
		t.Fatal(err)
	}
	name := unique("ЖК Новое имя")
	if _, err := m.UpdateProject(ctx, core.UpdateProjectInput{ProjectID: p.ID, Name: name, City: "Казань", DeveloperID: &dev.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetProject(ctx, p.ID)
	if err != nil || got.Name != name || got.City != "Казань" || got.DeveloperID == nil || *got.DeveloperID != dev.ID || !got.CreatedAt.Equal(p.CreatedAt) {
		t.Fatalf("project = %+v, %v", got, err)
	}
	if _, err := m.UpdateProject(ctx, core.UpdateProjectInput{ProjectID: p.ID, Name: name}); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.GetProject(ctx, p.ID); got.City != "" || got.DeveloperID != nil {
		t.Fatalf("city and developer must be cleared: %+v", got)
	}
	_, err = m.UpdateProject(ctx, core.UpdateProjectInput{ProjectID: p.ID, Name: "  "})
	mustRule(t, err, "required")
	missing := core.ID{}
	missing[0] = 1
	if _, err := m.UpdateProject(ctx, core.UpdateProjectInput{ProjectID: p.ID, Name: name, DeveloperID: &missing}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown developer: %v", err)
	}
}

// TestCoreUIFixes checks the pages for lots without a number, prices on
// request, incoming relations and project editing.
func TestCoreUIFixes(t *testing.T) {
	c, m := newUI(t)
	manual := manualSource(t, m)
	observedAt := time.Now().Add(-2 * time.Hour).In(time.FixedZone("MSK", 3*3600)).Format("2006-01-02T15:04")

	projectPath := c.mustRedirect("/projects", url.Values{"name": {unique("ЖК Элит UI")}})
	c.mustRedirect(projectPath+"/buildings", url.Values{"name": {"Корпус 1"}})
	buildingID := regexp.MustCompile(`value="b:([0-9a-f-]{36})"`).FindStringSubmatch(mustGet(t, c, projectPath))
	if buildingID == nil {
		t.Fatal("building is not offered for lots")
	}
	lotForm := url.Values{
		"placement": {"b:" + buildingID[1]}, "lot_type": {"apartment"}, "number": {""}, "status": {"available"},
		"area_total": {"150"}, "price": {""}, "observed_at": {observedAt}, "data_source_id": {manual.ID.String()},
	}

	// Two lots without a number and without a price in one building.
	first := c.mustRedirect(projectPath+"/lots", lotForm)
	onRequest := cloneValues(lotForm)
	onRequest.Set("price_on_request", "1")
	second := c.mustRedirect(projectPath+"/lots", onRequest)
	if first == second {
		t.Fatal("lots without a number must not be merged")
	}
	page := mustGet(t, c, projectPath)
	if strings.Count(page, "Не указан<") != 2 || !strings.Contains(page, "Цена по запросу</td>") || !strings.Contains(page, "Не указана</td>") {
		t.Fatal("project card must show lots without number and without price")
	}
	if card := mustGet(t, c, second); !strings.Contains(card, "номер не указан") || !strings.Contains(card, "<strong>Цена по запросу</strong>") || !strings.Contains(card, `value="1" checked`) {
		t.Fatal("lot card must show price on request and keep it in the observation form")
	}
	if card := mustGet(t, c, first); !strings.Contains(card, "<strong>Не указана</strong>") {
		t.Fatal("lot card must show a missing price as not stated")
	}
	withPrice := cloneValues(onRequest)
	withPrice.Set("price", "100 000 000")
	if status, _, body := c.post(projectPath+"/lots", withPrice); status != http.StatusUnprocessableEntity || !strings.Contains(body, "исключает цену") {
		t.Fatalf("price with price on request: status %d, alert %q", status, alertText(body))
	}

	// A relation stated on B is visible on A.
	otherName := unique("ЖК Сосед UI")
	otherPath := c.mustRedirect("/projects", url.Values{"name": {otherName}})
	projectID := strings.TrimPrefix(projectPath, "/projects/")
	c.mustRedirect(otherPath+"/relations", url.Values{"related_project_id": {projectID}, "relation_type": {"direct_competitor"}, "valid_from": {"2026-01-01"}})
	page = mustGet(t, c, projectPath)
	if !strings.Contains(page, "Проект указан у других проектов") || !strings.Contains(page, `href="`+otherPath+`">`+otherName) {
		t.Fatal("the competitor relation must be shown on the related project")
	}

	// Project editing.
	newName := unique("ЖК Переименован UI")
	c.mustRedirect(projectPath+"/edit", url.Values{"name": {newName}, "city": {"Сочи"}, "developer_id": {""}})
	page = mustGet(t, c, projectPath)
	if !strings.Contains(page, "<h1>"+newName) || !strings.Contains(page, "Сочи") {
		t.Fatal("project parameters are not updated")
	}
	if status, _, body := c.post(projectPath+"/edit", url.Values{"name": {" "}}); status != http.StatusUnprocessableEntity || !strings.Contains(body, "Заполните поле «Название»") {
		t.Fatalf("empty project name: status %d, alert %q", status, alertText(body))
	}
}
