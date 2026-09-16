//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"realtyradar/src/app"
	"realtyradar/src/modules/core"
	"realtyradar/src/platform/migrate"
)

// newCore applies all migrations and returns the Core module on the test database.
func newCore(t *testing.T) (*core.Module, *pgxpool.Pool) {
	t.Helper()
	pool := connect(t)
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	provider, err := migrate.NewProvider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	m, err := core.New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), core.Nav())
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	return m, pool
}

func unique(prefix string) string { return fmt.Sprintf("%s %d", prefix, time.Now().UnixNano()) }

func rub(v int64) core.Money { return core.Money(v * 100) }

func mustRule(t *testing.T, err error, code string) {
	t.Helper()
	re, ok := core.AsRuleError(err)
	if !ok || re.Code != code {
		t.Fatalf("error = %v, want rule error %q", err, code)
	}
}

func manualSource(t *testing.T, m *core.Module) core.DataSource {
	t.Helper()
	ds, err := m.DataSourceByCode(context.Background(), core.ManualDataSourceCode)
	if err != nil {
		t.Fatalf("manual data source must be seeded by the migration: %v", err)
	}
	return ds
}

func newProject(t *testing.T, m *core.Module) core.Project {
	t.Helper()
	p, err := m.CreateProject(context.Background(), core.CreateProjectInput{Name: unique("ЖК Тест"), City: "Москва"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func apartmentState(rooms int, area core.Area, price int64) core.ObservedState {
	return core.ObservedState{
		Status: core.Value(core.LotStatusAvailable), Rooms: core.Value(rooms),
		AreaTotal: core.Value(area), Price: core.Value(rub(price)),
	}
}

func TestCoreProjectHierarchy(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)

	dev, err := m.CreateDeveloper(ctx, unique("Девелопер"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := m.CreateProject(ctx, core.CreateProjectInput{Name: unique("ЖК Иерархия"), DeveloperID: &dev.ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.GetProject(ctx, p.ID)
	if err != nil || got.DeveloperID == nil || *got.DeveloperID != dev.ID {
		t.Fatalf("project developer not stored: %+v, %v", got, err)
	}

	b1, err := m.AddBuilding(ctx, p.ID, "Корпус 1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddBuilding(ctx, p.ID, "Корпус 1"); err == nil {
		t.Fatal("duplicate building name accepted")
	} else {
		mustRule(t, err, "name_taken")
	}
	if _, err := m.AddBuilding(ctx, p.ID, "Корпус 2"); err != nil {
		t.Fatal(err)
	}
	s1, err := m.AddSection(ctx, b1.ID, "1")
	if err != nil {
		t.Fatal(err)
	}
	views, err := m.ListBuildings(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || views[0].Building.ID != b1.ID || len(views[0].Sections) != 1 || views[0].Sections[0].ID != s1.ID || len(views[1].Sections) != 0 {
		t.Fatalf("unexpected structure: %+v", views)
	}

	if err := m.SetProjectDeveloper(ctx, p.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.GetProject(ctx, p.ID); got.DeveloperID != nil {
		t.Error("developer was not cleared")
	}
	if _, err := m.GetProject(ctx, core.ID{1}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("missing project error = %v, want ErrNotFound", err)
	}
}

func TestCoreLotsWithOptionalHierarchyLevels(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	other := newProject(t, m)
	otherBuilding, err := m.AddBuilding(ctx, other.ID, "Корпус Ч")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Hour)

	res, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: now, ProjectID: p.ID,
		Buildings: []core.BuildingDecision{{Ref: "b", NewName: "Корпус 1"}},
		Sections:  []core.SectionDecision{{Ref: "s", BuildingRef: "b", NewName: "2"}},
		Items: []core.LotObservation{
			{Ref: "parking", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeParking, Number: "М-1"}},
				State: core.ObservedState{Price: core.Value(rub(1_500_000))}},
			{Ref: "no-section", Target: core.LotTarget{NewLot: &core.NewLotPlacement{BuildingRef: "b", Type: core.LotTypeApartment, Number: "1"}},
				State: apartmentState(1, 3500, 9_000_000)},
			{Ref: "full", Target: core.LotTarget{NewLot: &core.NewLotPlacement{BuildingRef: "b", SectionRef: "s", Type: core.LotTypeApartment, Number: "2"}},
				State: apartmentState(2, 5500, 12_000_000)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := m.ListLots(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	byNumber := map[string]core.LotRow{}
	for _, r := range rows {
		byNumber[r.Lot.Placement.Number] = r
	}
	if r := byNumber["М-1"]; r.Lot.Placement.BuildingID != nil || r.Lot.Placement.SectionID != nil || r.Current == nil || r.Current.State.Status != core.LotStatusUnknown {
		t.Errorf("lot without building: %+v", r)
	}
	if r := byNumber["1"]; r.Lot.Placement.BuildingID == nil || r.Lot.Placement.SectionID != nil || r.BuildingName != "Корпус 1" {
		t.Errorf("lot without section: %+v", r)
	}
	if r := byNumber["2"]; r.SectionName != "2" || r.Current == nil || *r.Current.State.Price != rub(12_000_000) {
		t.Errorf("lot with section: %+v", r)
	}
	if len(res.Items) != 3 || !res.Items[0].LotIsNew || res.Items[0].Change != core.ChangeCreated {
		t.Errorf("unexpected batch result: %+v", res)
	}

	// A section without a building is refused, and nothing of the batch is written.
	_, err = m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: now, ProjectID: p.ID,
		Buildings: []core.BuildingDecision{{Ref: "b2", NewName: "Корпус 3"}},
		Sections:  []core.SectionDecision{{Ref: "s2", BuildingRef: "b2", NewName: "1"}},
		Items: []core.LotObservation{
			{Ref: "ok", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeStorage, Number: "К-1"}}},
			{Ref: "bad", Target: core.LotTarget{NewLot: &core.NewLotPlacement{SectionRef: "s2", Type: core.LotTypeApartment, Number: "3"}}},
		},
	})
	mustRule(t, err, "section_without_building")
	var itemErr *core.ItemError
	if !errors.As(err, &itemErr) || itemErr.Ref != "bad" {
		t.Errorf("error does not locate the item: %v", err)
	}
	if rows, _ := m.ListLots(ctx, p.ID); len(rows) != 3 {
		t.Errorf("failed batch must not write anything, lots = %d", len(rows))
	}
	if views, _ := m.ListBuildings(ctx, p.ID); len(views) != 1 {
		t.Errorf("failed batch must not create buildings, buildings = %d", len(views))
	}

	// A building of another project is refused.
	_, err = m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: now, ProjectID: p.ID,
		Buildings: []core.BuildingDecision{{Ref: "x", ExistingID: &otherBuilding.ID}},
		Items:     []core.LotObservation{{Ref: "x1", Target: core.LotTarget{NewLot: &core.NewLotPlacement{BuildingRef: "x", Type: core.LotTypeApartment, Number: "9"}}}},
	})
	mustRule(t, err, "wrong_project")

	// Registering the same placement again is refused (no silent duplicates).
	_, err = m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: now, ProjectID: p.ID,
		Items: []core.LotObservation{{Ref: "dup", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeParking, Number: "М-1"}}}},
	})
	mustRule(t, err, "lot_already_exists")
}

func TestCoreLotHistory(t *testing.T) {
	ctx := context.Background()
	m, pool := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)

	t1 := time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)
	t2 := t1.Add(7 * 24 * time.Hour)
	t3 := t1.Add(11 * 24 * time.Hour)
	batch := func(at time.Time, items ...core.LotObservation) core.ObservationBatch {
		return core.ObservationBatch{DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID, Items: items}
	}

	// Observation 1: lot 145, 15 900 000.
	floor := 7
	r1, err := m.IngestObservationBatch(ctx, batch(t1, core.LotObservation{
		Ref:    "145",
		Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "145", Floor: &floor}},
		State:  apartmentState(2, 6740, 15_900_000),
	}))
	if err != nil {
		t.Fatal(err)
	}
	lotID := r1.Items[0].LotID
	existing := func(st core.ObservedState) core.LotObservation {
		return core.LotObservation{Ref: "145", Target: core.LotTarget{ExistingLotID: &lotID}, State: st}
	}

	// The same values again: no new version.
	r2, err := m.IngestObservationBatch(ctx, batch(t2, existing(apartmentState(2, 6740, 15_900_000))))
	if err != nil {
		t.Fatal(err)
	}
	if r2.Items[0].Change != core.ChangeConfirmed || r2.Items[0].VersionID != r1.Items[0].VersionID {
		t.Fatalf("identical observation must confirm the current version: %+v", r2.Items[0])
	}
	h, err := m.GetLotHistory(ctx, lotID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Versions) != 1 || !h.Versions[0].LastObservedAt.Equal(t2) || !h.Primary.Listing.LastSeenAt.Equal(t2) || !h.Primary.Listing.FirstSeenAt.Equal(t1) {
		t.Fatalf("after identical observation: versions=%+v primary=%+v", h.Versions, h.Primary)
	}

	// Observation 2: price 16 400 000 → new version.
	r3, err := m.IngestObservationBatch(ctx, batch(t3, existing(core.ObservedState{Price: core.Value(rub(16_400_000))})))
	if err != nil {
		t.Fatal(err)
	}
	if r3.Items[0].Change != core.ChangeChanged {
		t.Fatalf("changed observation: %+v", r3.Items[0])
	}
	h, err = m.GetLotHistory(ctx, lotID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(h.Versions))
	}
	cur := h.Current()
	if cur == nil || *cur.State.Price != rub(16_400_000) || *cur.State.Rooms != 2 || *cur.State.AreaTotal != 6740 || !cur.ValidFrom.Equal(t3) {
		t.Fatalf("current state: %+v", cur)
	}
	if prev := h.Versions[0]; prev.ValidTo == nil || !prev.ValidTo.Equal(t3) || *prev.State.Price != rub(15_900_000) {
		t.Fatalf("previous state: %+v", prev)
	}

	for _, tt := range []struct {
		name string
		at   time.Time
		want *core.Money
	}{
		{"before first observation", t1.Add(-time.Minute), nil},
		{"first observation", t1, ptrMoney(rub(15_900_000))},
		{"between", t2.Add(time.Hour), ptrMoney(rub(15_900_000))},
		{"change moment", t3, ptrMoney(rub(16_400_000))},
		{"now", time.Now(), ptrMoney(rub(16_400_000))},
	} {
		t.Run("state at "+tt.name, func(t *testing.T) {
			v, err := m.GetLotState(ctx, lotID, tt.at)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == nil {
				if v != nil {
					t.Fatalf("want no state, got %+v", v)
				}
				return
			}
			if v == nil || *v.State.Price != *tt.want {
				t.Fatalf("state = %+v, want price %d", v, *tt.want)
			}
		})
	}

	// An observation earlier than the current version is refused (backfill is deferred).
	_, err = m.IngestObservationBatch(ctx, batch(t2, existing(apartmentState(2, 6740, 15_000_000))))
	mustRule(t, err, "observation_out_of_order")

	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.lot_listing_versions v JOIN core.lot_listings l ON l.id = v.listing_id WHERE l.lot_id = $1`,
		lotID.String()).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 2 {
		t.Errorf("stored versions = %d, want 2", versions)
	}
}

func ptrMoney(m core.Money) *core.Money { return &m }

func TestCorePriceValidation(t *testing.T) {
	ctx := context.Background()
	m, pool := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	at := time.Now().Add(-time.Hour)

	tests := []struct {
		name  string
		state core.ObservedState
		code  string
	}{
		{"zero price", core.ObservedState{Price: core.Value(core.Money(0))}, "must_be_positive"},
		{"negative price", core.ObservedState{Price: core.Value(rub(-5))}, "must_be_positive"},
		{"price before discount equal to price", core.ObservedState{Price: core.Value(rub(10)), PriceBeforeDiscount: core.Value(rub(10))}, "price_before_discount_not_greater"},
		{"price before discount below price", core.ObservedState{Price: core.Value(rub(10)), PriceBeforeDiscount: core.Value(rub(9))}, "price_before_discount_not_greater"},
		{"price before discount without price", core.ObservedState{PriceBeforeDiscount: core.Value(rub(10))}, "price_before_discount_without_price"},
		{"zero area", core.ObservedState{AreaTotal: core.Value(core.Area(0))}, "must_be_positive"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
				DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID,
				Items: []core.LotObservation{{Ref: "x", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: fmt.Sprint(100 + i)}}, State: tt.state}},
			})
			mustRule(t, err, tt.code)
		})
	}

	res, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID,
		Items: []core.LotObservation{{Ref: "ok", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "200"}},
			State: core.ObservedState{Price: core.Value(rub(10_000_000)), PriceBeforeDiscount: core.Value(rub(10_500_000))}}},
	})
	if err != nil {
		t.Fatalf("valid discount rejected: %v", err)
	}
	h, _ := m.GetLotHistory(ctx, res.Items[0].LotID)
	if d := h.Current().State.Discount(); d == nil || *d != rub(500_000) {
		t.Errorf("discount = %v, want 500000.00", d)
	}

	// The database refuses the same invariant even if the application is bypassed.
	_, err = pool.Exec(ctx, `UPDATE core.lot_listing_versions SET price_before_discount = price WHERE id = $1`, res.Items[0].VersionID.String())
	assertPgCode(t, err, "23514") // check_violation
}

func TestCoreBatchIdempotency(t *testing.T) {
	ctx := context.Background()
	m, pool := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	ref := newOriginRef(t)
	b := core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginDocumentImport, OriginRef: &ref,
		ObservedAt: time.Now().Add(-time.Hour), ObservedAtBasis: core.BasisDocumentDate, ProjectID: p.ID,
		Items: []core.LotObservation{{Ref: "1", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "1"}}, State: apartmentState(1, 3000, 8_000_000)}},
	}
	first, err := m.IngestObservationBatch(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.IngestObservationBatch(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if !again.AlreadyApplied || again.BatchID != first.BatchID {
		t.Fatalf("repeated batch: %+v", again)
	}
	if rows, _ := m.ListLots(ctx, p.ID); len(rows) != 1 {
		t.Errorf("repeated batch created lots: %d", len(rows))
	}

	// The batch header keeps the provenance of the observation (review M1).
	var basis, completeness string
	var hash []byte
	var confirmed *time.Time
	if err := pool.QueryRow(ctx, `SELECT observed_at_basis, completeness, content_hash, confirmed_at
		FROM core.ingestion_batches WHERE id = $1`, first.BatchID.String()).Scan(&basis, &completeness, &hash, &confirmed); err != nil {
		t.Fatal(err)
	}
	if basis != "document_date" || completeness != "partial" || len(hash) != 32 || confirmed != nil {
		t.Errorf("batch header: basis=%q completeness=%q hash=%d bytes confirmed=%v", basis, completeness, len(hash), confirmed)
	}

	// The same origin reference with different content is a conflict, not a repeat.
	changed := b
	changed.Items = []core.LotObservation{{Ref: "1", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "1"}},
		State: apartmentState(1, 3000, 9_500_000)}}
	_, err = m.IngestObservationBatch(ctx, changed)
	mustRule(t, err, "origin_conflict")
	if rows, _ := m.ListLots(ctx, p.ID); len(rows) != 1 {
		t.Errorf("refused batch created lots: %d", len(rows))
	}

	// A different origin reference is a different batch.
	other := b
	otherRef := newOriginRef(t)
	other.OriginRef = &otherRef
	other.Items = []core.LotObservation{{Ref: "5", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "5"}},
		State: apartmentState(1, 3000, 8_000_000)}}
	second, err := m.IngestObservationBatch(ctx, other)
	if err != nil || second.AlreadyApplied || second.BatchID == first.BatchID {
		t.Fatalf("second import: %+v, %v", second, err)
	}

	// A document import must state what its moment is based on.
	noBasis := b
	noBasis.OriginRef, noBasis.ObservedAtBasis = nil, ""
	_, err = m.IngestObservationBatch(ctx, noBasis)
	mustRule(t, err, "required")

	// Confirmation and scope reach the stored header.
	confirmedAt := time.Now().Add(-time.Minute)
	scopedRef := newOriginRef(t)
	scoped := b
	scoped.OriginRef, scoped.ConfirmedAt = &scopedRef, &confirmedAt
	scoped.Scope = core.BatchScope{LotTypes: []core.LotType{core.LotTypeApartment}}
	scoped.Items = []core.LotObservation{{Ref: "2", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "2"}}, State: apartmentState(1, 3000, 8_000_000)}}
	res, err := m.IngestObservationBatch(ctx, scoped)
	if err != nil {
		t.Fatal(err)
	}
	var lotTypes []string
	if err := pool.QueryRow(ctx, `SELECT scope_lot_types, confirmed_at FROM core.ingestion_batches WHERE id = $1`,
		res.BatchID.String()).Scan(&lotTypes, &confirmed); err != nil {
		t.Fatal(err)
	}
	if len(lotTypes) != 1 || lotTypes[0] != "apartment" || confirmed == nil || !confirmed.Equal(confirmedAt.UTC().Truncate(time.Microsecond)) {
		t.Errorf("scope = %v, confirmed_at = %v", lotTypes, confirmed)
	}
}

// newOriginRef returns an identifier of an import in a sending module.
func newOriginRef(t *testing.T) core.ID {
	t.Helper()
	id, err := core.ParseID(fmt.Sprintf("01a0a5df-0000-7000-8000-%012d", time.Now().UnixNano()%1_000_000_000_000))
	if err != nil {
		t.Fatalf("origin ref: %v", err)
	}
	return id
}

func TestCoreRelationsAndPortfolio(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	own := newProject(t, m)
	competitor := newProject(t, m)
	reference := newProject(t, m)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if _, err := m.AddProjectToPortfolio(ctx, core.AddToPortfolioInput{ProjectID: own.ID, ValidFrom: &jan1}); err != nil {
		t.Fatal(err)
	}
	_, err := m.AddProjectToPortfolio(ctx, core.AddToPortfolioInput{ProjectID: own.ID})
	mustRule(t, err, "period_overlap")

	portfolio, err := m.ListPortfolioProjects(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	inPortfolio := map[core.ID]bool{}
	for _, s := range portfolio {
		inPortfolio[s.Project.ID] = true
	}
	if !inPortfolio[own.ID] || inPortfolio[competitor.ID] {
		t.Errorf("portfolio membership is wrong: own=%v competitor=%v", inPortfolio[own.ID], inPortfolio[competitor.ID])
	}
	if old, _ := m.ListPortfolioProjects(ctx, time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)); containsProject(old, own.ID) {
		t.Error("project must not be in the portfolio before its start date")
	}

	if _, err := m.EstablishProjectRelation(ctx, core.RelationInput{SubjectProjectID: own.ID, RelatedProjectID: competitor.ID, Type: core.RelationDirectCompetitor, ValidFrom: jan1}); err != nil {
		t.Fatal(err)
	}
	// The same external project is a reference for another project, and the relation is directed.
	if _, err := m.EstablishProjectRelation(ctx, core.RelationInput{SubjectProjectID: reference.ID, RelatedProjectID: competitor.ID, Type: core.RelationReference, ValidFrom: jan1, Rationale: "благоустройство"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EstablishProjectRelation(ctx, core.RelationInput{SubjectProjectID: own.ID, RelatedProjectID: reference.ID, Type: core.RelationReference, ValidFrom: jan1}); err != nil {
		t.Fatal(err)
	}
	_, err = m.EstablishProjectRelation(ctx, core.RelationInput{SubjectProjectID: own.ID, RelatedProjectID: competitor.ID, Type: core.RelationDirectCompetitor, ValidFrom: jan1.AddDate(0, 3, 0)})
	mustRule(t, err, "period_overlap")
	_, err = m.EstablishProjectRelation(ctx, core.RelationInput{SubjectProjectID: own.ID, RelatedProjectID: own.ID, Type: core.RelationReference, ValidFrom: jan1})
	mustRule(t, err, "self_relation")

	relations, err := m.ListProjectRelations(ctx, own.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 2 {
		t.Fatalf("relations of own project = %d, want 2", len(relations))
	}
	types := map[core.ID]core.RelationType{}
	for _, r := range relations {
		types[r.RelatedProject.ID] = r.Relation.Type
		if !r.Active {
			t.Errorf("relation %s must be active", r.Relation.ID)
		}
	}
	if types[competitor.ID] != core.RelationDirectCompetitor || types[reference.ID] != core.RelationReference {
		t.Errorf("relation types = %v", types)
	}
	if inbound, _ := m.ListProjectRelations(ctx, competitor.ID, time.Now()); len(inbound) != 0 {
		t.Errorf("relations are directed; competitor must have no own relations, got %d", len(inbound))
	}
}

func containsProject(list []core.ProjectSummary, id core.ID) bool {
	for _, s := range list {
		if s.Project.ID == id {
			return true
		}
	}
	return false
}

func TestCoreSchemaKeepsHierarchyConsistent(t *testing.T) {
	ctx := context.Background()
	m, pool := newCore(t)
	p1, p2 := newProject(t, m), newProject(t, m)
	b2, err := m.AddBuilding(ctx, p2.ID, "Корпус")
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO core.lots (id, project_id, building_id, lot_type, number, created_at, updated_at)
		VALUES ($1, $2, $3, 'apartment', '1', now(), now())`,
		"01a0a5df-0000-7000-8000-00000000abcd", p1.ID.String(), b2.ID.String())
	assertPgCode(t, err, "23503") // foreign_key_violation: building of another project
}

func TestDemoData(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	loaded, err := app.LoadDemoData(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	again, err := app.LoadDemoData(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Error("demo data must load only once")
	}
	if !loaded {
		t.Log("demo data was already present in the test database")
	}
	projects, err := m.ListProjects(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var alpha *core.ProjectSummary
	for i := range projects {
		if projects[i].Project.Name == app.DemoProjectName {
			alpha = &projects[i]
		}
	}
	if alpha == nil || !alpha.InPortfolio || alpha.LotCount != 2 {
		t.Fatalf("demo company project: %+v", alpha)
	}
	lots, err := m.ListLots(ctx, alpha.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lots {
		if l.Lot.Placement.Number != "145" {
			continue
		}
		h, err := m.GetLotHistory(ctx, l.Lot.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.Versions) != 2 || *h.Versions[0].State.Price != rub(15_900_000) || *h.Current().State.Price != rub(16_400_000) {
			t.Fatalf("demo lot 145 history: %+v", h.Versions)
		}
		return
	}
	t.Fatal("demo lot 145 not found")
}

func assertPgCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("error = %v, want PostgreSQL error %s", err, code)
	}
}
