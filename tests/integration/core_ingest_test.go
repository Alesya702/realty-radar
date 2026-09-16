//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"realtyradar/src/modules/core"
)

// existingBuilding returns the batch decision "this building of the project".
func existingBuilding(ref string, id core.ID) core.BuildingDecision {
	return core.BuildingDecision{Ref: ref, ExistingID: &id}
}

// TestCoreLotMatchingKeyK2 checks the duplicate check of a confirmed "new lot"
// decision against key K2 (project, building, lot type, number): the section is
// not part of the key, and an unknown building makes the match ambiguous
// instead of creating a second lot silently (review H3).
func TestCoreLotMatchingKeyK2(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	at := time.Now().Add(-time.Hour)

	b1, err := m.AddBuilding(ctx, p.ID, "Корпус 1")
	if err != nil {
		t.Fatal(err)
	}
	b2, err := m.AddBuilding(ctx, p.ID, "Корпус 2")
	if err != nil {
		t.Fatal(err)
	}
	s1, err := m.AddSection(ctx, b1.ID, "2")
	if err != nil {
		t.Fatal(err)
	}

	ingest := func(t *testing.T, ref string, buildings []core.BuildingDecision, sections []core.SectionDecision, np *core.NewLotPlacement) (core.BatchResult, error) {
		t.Helper()
		return m.IngestObservationBatch(ctx, core.ObservationBatch{
			DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID,
			Buildings: buildings, Sections: sections,
			Items: []core.LotObservation{{Ref: ref, Target: core.LotTarget{NewLot: np}, State: apartmentState(2, 6740, 15_900_000)}},
		})
	}

	// Lot 145 exists in building 1, section 2.
	res, err := ingest(t, "145",
		[]core.BuildingDecision{existingBuilding("b", b1.ID)},
		[]core.SectionDecision{{Ref: "s", ExistingID: &s1.ID}},
		&core.NewLotPlacement{BuildingRef: "b", SectionRef: "s", Type: core.LotTypeApartment, Number: "145"})
	if err != nil {
		t.Fatal(err)
	}
	lot145 := res.Items[0].LotID

	t.Run("same building without the section is the same lot", func(t *testing.T) {
		_, err := ingest(t, "145-no-section", []core.BuildingDecision{existingBuilding("b", b1.ID)}, nil,
			&core.NewLotPlacement{BuildingRef: "b", Type: core.LotTypeApartment, Number: "145"})
		mustRule(t, err, "lot_already_exists")
	})

	t.Run("unknown building makes the match ambiguous", func(t *testing.T) {
		_, err := ingest(t, "145-no-building", nil, nil,
			&core.NewLotPlacement{Type: core.LotTypeApartment, Number: "145"})
		mustRule(t, err, "match_conflict")
	})

	t.Run("the same number in another building is another lot", func(t *testing.T) {
		res, err := ingest(t, "145-b2", []core.BuildingDecision{existingBuilding("b", b2.ID)}, nil,
			&core.NewLotPlacement{BuildingRef: "b", Type: core.LotTypeApartment, Number: "145"})
		if err != nil {
			t.Fatalf("a lot with the same number in another building must be accepted: %v", err)
		}
		if res.Items[0].LotID == lot145 {
			t.Error("the lot of another building must be a new lot")
		}
	})

	t.Run("another lot type with the same number is another lot", func(t *testing.T) {
		if _, err := ingest(t, "145-parking", []core.BuildingDecision{existingBuilding("b", b1.ID)}, nil,
			&core.NewLotPlacement{BuildingRef: "b", Type: core.LotTypeParking, Number: "145"}); err != nil {
			t.Fatalf("a parking lot with the same number must be accepted: %v", err)
		}
	})

	// A lot registered without a building: a later document that does state the
	// building must not create a second lot silently.
	if _, err := ingest(t, "300", nil, nil, &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "300"}); err != nil {
		t.Fatal(err)
	}
	t.Run("stored lot without a building is a possible match", func(t *testing.T) {
		_, err := ingest(t, "300-b1", []core.BuildingDecision{existingBuilding("b", b1.ID)}, nil,
			&core.NewLotPlacement{BuildingRef: "b", Type: core.LotTypeApartment, Number: "300"})
		mustRule(t, err, "match_conflict")
	})

	// No refused decision created a lot.
	rows, err := m.ListLots(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	byNumber := map[string]int{}
	for _, r := range rows {
		byNumber[r.Lot.Placement.Number]++
	}
	if byNumber["145"] != 3 || byNumber["300"] != 1 || len(rows) != 4 {
		t.Errorf("lots = %d (%v), want 145 three times (two buildings + parking) and 300 once", len(rows), byNumber)
	}
}

// TestCoreBatchReportsAllItemErrors checks that one confirmation reports every
// item-level violation and writes nothing (review H4).
func TestCoreBatchReportsAllItemErrors(t *testing.T) {
	ctx := context.Background()
	m, pool := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	at := time.Now().Add(-time.Hour)

	newLot := func(number string) core.LotTarget {
		return core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: number}}
	}
	batches := countRows(t, pool, `SELECT count(*) FROM core.ingestion_batches`)

	_, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID,
		Items: []core.LotObservation{
			{Ref: "ok-1", Target: newLot("1"), State: apartmentState(1, 3000, 8_000_000)},
			{Ref: "zero-price", Target: newLot("2"), State: core.ObservedState{Price: core.Value(core.Money(0))}},
			{Ref: "ok-2", Target: newLot("3"), State: apartmentState(2, 6000, 12_000_000)},
			{Ref: "discount", Target: newLot("4"), State: core.ObservedState{Price: core.Value(rub(10)), PriceBeforeDiscount: core.Value(rub(9))}},
			{Ref: "no-target", Target: core.LotTarget{}, State: apartmentState(1, 3000, 8_000_000)},
			{Ref: "zero-area", Target: newLot("6"), State: core.ObservedState{AreaTotal: core.Value(core.Area(0))}},
		},
	})

	var batchErr *core.BatchError
	if !errors.As(err, &batchErr) {
		t.Fatalf("error = %v (%T), want *core.BatchError", err, err)
	}
	got := map[string]string{}
	for _, item := range batchErr.Items {
		re, ok := core.AsRuleError(item.Err)
		if !ok {
			t.Fatalf("item %q: error is not a rule violation: %v", item.Ref, item.Err)
		}
		got[item.Ref] = re.Code
	}
	want := map[string]string{
		"zero-price": "must_be_positive",
		"discount":   "price_before_discount_not_greater",
		"no-target":  "invalid_target",
		"zero-area":  "must_be_positive",
	}
	if len(got) != len(want) {
		t.Fatalf("reported items = %v, want %v", got, want)
	}
	for ref, code := range want {
		if got[ref] != code {
			t.Errorf("item %q: code %q, want %q", ref, got[ref], code)
		}
	}

	// Nothing of the refused batch is written.
	if rows, _ := m.ListLots(ctx, p.ID); len(rows) != 0 {
		t.Errorf("refused batch created %d lots", len(rows))
	}
	if now := countRows(t, pool, `SELECT count(*) FROM core.ingestion_batches`); now != batches {
		t.Errorf("ingestion batches = %d, want %d: the batch header must be rolled back too", now, batches)
	}

	// The same batch without the bad items is applied as a whole.
	res, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID,
		Items: []core.LotObservation{
			{Ref: "ok-1", Target: newLot("1"), State: apartmentState(1, 3000, 8_000_000)},
			{Ref: "ok-2", Target: newLot("3"), State: apartmentState(2, 6000, 12_000_000)},
		},
	})
	if err != nil || len(res.Items) != 2 {
		t.Fatalf("valid batch: %+v, %v", res, err)
	}
}

// TestCoreDuplicateInsideOneBatch checks that two items describing one lot are
// reported as a duplicate of the batch, not as a collision with a stored lot
// (review L11).
func TestCoreDuplicateInsideOneBatch(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	at := time.Now().Add(-time.Hour)
	b1, err := m.AddBuilding(ctx, p.ID, "Корпус 1")
	if err != nil {
		t.Fatal(err)
	}
	s1, err := m.AddSection(ctx, b1.ID, "1")
	if err != nil {
		t.Fatal(err)
	}

	ingest := func(items ...core.LotObservation) error {
		_, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
			DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID,
			Buildings: []core.BuildingDecision{existingBuilding("b", b1.ID)},
			Sections:  []core.SectionDecision{{Ref: "s", ExistingID: &s1.ID}},
			Items:     items,
		})
		return err
	}

	newLot := func(sectionRef, number string) core.LotTarget {
		return core.LotTarget{NewLot: &core.NewLotPlacement{BuildingRef: "b", SectionRef: sectionRef, Type: core.LotTypeApartment, Number: number}}
	}
	state := apartmentState(1, 3000, 8_000_000)

	err = ingest(
		core.LotObservation{Ref: "first", Target: newLot("", "10"), State: state},
		core.LotObservation{Ref: "second", Target: newLot("", "10"), State: state},
	)
	mustRule(t, err, "duplicate_in_batch")
	var batchErr *core.BatchError
	if !errors.As(err, &batchErr) || len(batchErr.Items) != 1 || batchErr.Items[0].Ref != "second" {
		t.Fatalf("error must locate the second item: %v", err)
	}

	// The section is not part of the identity, so these two are the same lot too.
	err = ingest(
		core.LotObservation{Ref: "first", Target: newLot("", "11"), State: state},
		core.LotObservation{Ref: "second", Target: newLot("s", "11"), State: state},
	)
	mustRule(t, err, "duplicate_in_batch")

	// Two items about the same existing lot are a duplicate as well.
	res, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginManualEntry, ObservedAt: at, ProjectID: p.ID,
		Items: []core.LotObservation{{Ref: "12", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "12"}}, State: state}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lotID := res.Items[0].LotID
	err = ingest(
		core.LotObservation{Ref: "a", Target: core.LotTarget{ExistingLotID: &lotID}, State: state},
		core.LotObservation{Ref: "b", Target: core.LotTarget{ExistingLotID: &lotID}, State: state},
	)
	mustRule(t, err, "duplicate_in_batch")

	if rows, _ := m.ListLots(ctx, p.ID); len(rows) != 1 {
		t.Errorf("lots = %d, want only the one created by the valid batch", len(rows))
	}
}

// TestCoreConcurrentConfirmationOfOneImport checks that two simultaneous
// confirmations of the same import (a double click, a repeat by the sending
// module) end as one applied batch and one AlreadyApplied, not as a unique
// violation (review M4).
func TestCoreConcurrentConfirmationOfOneImport(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	src := manualSource(t, m)
	p := newProject(t, m)
	ref := newOriginRef(t)
	b := core.ObservationBatch{
		DataSourceID: src.ID, OriginKind: core.OriginDocumentImport, OriginRef: &ref,
		ObservedAt: time.Now().Add(-time.Hour), ObservedAtBasis: core.BasisDocumentDate, ProjectID: p.ID,
		Items: []core.LotObservation{
			{Ref: "1", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "1"}}, State: apartmentState(1, 3000, 8_000_000)},
			{Ref: "2", Target: core.LotTarget{NewLot: &core.NewLotPlacement{Type: core.LotTypeApartment, Number: "2"}}, State: apartmentState(2, 6000, 12_000_000)},
		},
	}

	type outcome struct {
		res core.BatchResult
		err error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			res, err := m.IngestObservationBatch(ctx, b)
			results <- outcome{res, err}
		}()
	}
	close(start)

	applied, alreadyApplied := 0, 0
	var batchID core.ID
	for i := 0; i < 2; i++ {
		got := <-results
		if got.err != nil {
			t.Fatalf("confirmation %d failed: %v", i, got.err)
		}
		if got.res.AlreadyApplied {
			alreadyApplied++
		} else {
			applied++
		}
		if batchID.IsZero() {
			batchID = got.res.BatchID
		} else if got.res.BatchID != batchID {
			t.Errorf("confirmations report different batches: %s and %s", batchID, got.res.BatchID)
		}
	}
	if applied != 1 || alreadyApplied != 1 {
		t.Errorf("applied = %d, already applied = %d, want 1 and 1", applied, alreadyApplied)
	}
	if rows, _ := m.ListLots(ctx, p.ID); len(rows) != 2 {
		t.Errorf("lots = %d, want 2: the import must be applied once", len(rows))
	}
}

// TestCoreProjectLockAllowsReferencingInserts checks that the lock Core takes
// on a project does not conflict with the key share lock PostgreSQL takes when
// another transaction inserts a row referencing that project (review M5).
func TestCoreProjectLockAllowsReferencingInserts(t *testing.T) {
	ctx := context.Background()
	m, pool := newCore(t)
	a := newProject(t, m)

	// A transaction that holds exactly the lock an insert referencing the
	// project takes.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM core.projects WHERE id = $1 FOR KEY SHARE`, a.ID.String()); err != nil {
		t.Fatal(err)
	}

	waiting, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := m.AddBuilding(waiting, a.ID, unique("Корпус"))
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("add building while the project row is key-share locked: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the project lock blocks transactions that only reference the project")
	}
	_ = tx.Rollback(ctx)

	// Mutual relations established concurrently must not deadlock: each side
	// locks its own project and inserts a row referencing the other one. A
	// fresh pair per iteration keeps the relations free of period overlaps.
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 15; i++ {
		left, right := newProject(t, m), newProject(t, m)
		errs := make(chan error, 2)
		relate := func(subject, related core.ID) {
			_, err := m.EstablishProjectRelation(ctx, core.RelationInput{
				SubjectProjectID: subject, RelatedProjectID: related,
				Type: core.RelationDirectCompetitor, ValidFrom: jan1,
			})
			errs <- err
		}
		go relate(left.ID, right.ID)
		go relate(right.ID, left.ID)
		for j := 0; j < 2; j++ {
			if err := <-errs; err != nil {
				t.Fatalf("concurrent mutual relations (iteration %d): %v", i, err)
			}
		}
	}
}

// TestCoreBusinessDateOfPortfolio checks that the business date (A-07) is
// decided by Core, not by the zone of the moment a caller passes (review M6).
func TestCoreBusinessDateOfPortfolio(t *testing.T) {
	ctx := context.Background()
	m, _ := newCore(t)
	p := newProject(t, m)

	// The membership starts on 17 September (Moscow).
	from := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	if _, err := m.AddProjectToPortfolio(ctx, core.AddToPortfolioInput{ProjectID: p.ID, ValidFrom: &from}); err != nil {
		t.Fatal(err)
	}

	inPortfolio := func(at time.Time) bool {
		projects, err := m.ListPortfolioProjects(ctx, at)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range projects {
			if s.Project.ID == p.ID {
				return true
			}
		}
		return false
	}

	// 16 September 20:59 UTC is still 16 September in Moscow.
	if inPortfolio(time.Date(2026, 9, 16, 20, 59, 0, 0, time.UTC)) {
		t.Error("the project must not be in the portfolio before the Moscow date of the start")
	}
	// 16 September 21:30 UTC is already 17 September in Moscow.
	if !inPortfolio(time.Date(2026, 9, 16, 21, 30, 0, 0, time.UTC)) {
		t.Error("a UTC moment after the Moscow midnight must give the next business date")
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
