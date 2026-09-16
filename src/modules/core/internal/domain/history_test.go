package domain

import (
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func rub(v int64) Money { return Money(v * 100) }

func TestLotStateValidate(t *testing.T) {
	valid := func() LotState {
		return LotState{Presence: PresenceListed, Status: LotStatusAvailable, Rooms: ptr(2), AreaTotal: ptr(Area(6740)), Price: ptr(rub(15_900_000))}
	}
	tests := []struct {
		name     string
		mutate   func(*LotState)
		wantCode string
	}{
		{"valid", func(*LotState) {}, ""},
		{"all optional values empty", func(s *LotState) { s.Rooms, s.AreaTotal, s.Price = nil, nil, nil }, ""},
		{"zero price", func(s *LotState) { s.Price = ptr(Money(0)) }, CodeMustBePositive},
		{"negative price", func(s *LotState) { s.Price = ptr(rub(-1)) }, CodeMustBePositive},
		{"zero area", func(s *LotState) { s.AreaTotal = ptr(Area(0)) }, CodeMustBePositive},
		{"negative rooms", func(s *LotState) { s.Rooms = ptr(-1) }, CodeMustNotBeNegative},
		{"discount valid", func(s *LotState) { s.PriceBeforeDiscount = ptr(rub(16_400_000)) }, ""},
		{"price before discount equal to price", func(s *LotState) { s.PriceBeforeDiscount = ptr(rub(15_900_000)) }, CodeDiscountNotGreater},
		{"price before discount lower than price", func(s *LotState) { s.PriceBeforeDiscount = ptr(rub(15_000_000)) }, CodeDiscountNotGreater},
		{"price before discount without price", func(s *LotState) { s.Price = nil; s.PriceBeforeDiscount = ptr(rub(16_400_000)) }, CodeDiscountWithoutPrice},
		{"unknown status", func(s *LotState) { s.Status = "hidden" }, CodeInvalidValue},
		{"unknown presence", func(s *LotState) { s.Presence = "gone" }, CodeInvalidValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := valid()
			tt.mutate(&s)
			assertRuleCode(t, s.Validate(), tt.wantCode)
		})
	}
}

func TestLotStateCalculatedValues(t *testing.T) {
	s := LotState{Price: ptr(rub(15_900_000)), PriceBeforeDiscount: ptr(rub(16_400_000)), AreaTotal: ptr(Area(6740))}
	if got := s.Discount(); got == nil || *got != rub(500_000) {
		t.Errorf("Discount() = %v, want 500000.00", got)
	}
	if got := s.PricePerSqm(); got == nil || *got != Money(23590504) {
		t.Errorf("PricePerSqm() = %v, want 235905.04", got)
	}
	if (LotState{Price: ptr(rub(1))}).PricePerSqm() != nil {
		t.Error("PricePerSqm without area must be nil")
	}
}

// observedLot145 is the first observation of lot 145 from the MVP scenario.
func observedLot145(price int64) ObservedState {
	return ObservedState{
		Status:    Value(LotStatusAvailable),
		Rooms:     Value(2),
		AreaTotal: Value(Area(6740)),
		Price:     Value(rub(price)),
	}
}

func TestApplyObservationScenario(t *testing.T) {
	listing, batch := NewID(), NewID()
	t1 := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(24 * time.Hour)
	t3 := t1.Add(10 * 24 * time.Hour)

	// Observation 1: first state.
	c1, err := ApplyObservation(nil, observedLot145(15_900_000), t1, NewID(), listing, batch, t1)
	if err != nil {
		t.Fatal(err)
	}
	if c1.Kind != ChangeCreated || c1.Opened == nil || !c1.Opened.IsCurrent() {
		t.Fatalf("first observation: %+v", c1)
	}
	v1 := *c1.Opened

	// Same values again: no new version, last observation moves.
	c2, err := ApplyObservation(&v1, observedLot145(15_900_000), t2, NewID(), listing, batch, t2)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Kind != ChangeConfirmed || c2.Opened != nil || c2.Confirmed.LastObservedAt != t2 || c2.Confirmed.ValidFrom != t1 {
		t.Fatalf("identical observation must only confirm: %+v", c2)
	}
	v1 = *c2.Confirmed

	// Observation 2: changed price → new version.
	c3, err := ApplyObservation(&v1, observedLot145(16_400_000), t3, NewID(), listing, batch, t3)
	if err != nil {
		t.Fatal(err)
	}
	if c3.Kind != ChangeChanged || c3.Closed == nil || c3.Opened == nil {
		t.Fatalf("changed observation: %+v", c3)
	}
	if *c3.Closed.ValidTo != t3 || c3.Opened.ValidFrom != t3 || *c3.Opened.State.Price != rub(16_400_000) {
		t.Fatalf("wrong interval boundaries: closed=%+v opened=%+v", c3.Closed, c3.Opened)
	}

	history := []Version{*c3.Opened, *c3.Closed}
	SortVersions(history)
	tests := []struct {
		name string
		at   time.Time
		want *Money
	}{
		{"before first observation", t1.Add(-time.Second), nil},
		{"at first observation", t1, ptr(rub(15_900_000))},
		{"between observations", t2.Add(time.Hour), ptr(rub(15_900_000))},
		{"just before change", t3.Add(-time.Microsecond), ptr(rub(15_900_000))},
		{"at change", t3, ptr(rub(16_400_000))},
		{"long after", t3.Add(365 * 24 * time.Hour), ptr(rub(16_400_000))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := StateAt(history, tt.at)
			if tt.want == nil {
				if ok {
					t.Fatalf("expected no state, got %+v", v)
				}
				return
			}
			if !ok || *v.State.Price != *tt.want {
				t.Fatalf("StateAt = %+v, %v; want price %d", v, ok, *tt.want)
			}
		})
	}
}

func TestApplyObservationFieldStates(t *testing.T) {
	t1 := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	first := observedLot145(15_900_000)
	first.PriceBeforeDiscount = Value(rub(16_400_000))
	first.Finish = Value(" без отделки ")
	c, err := ApplyObservation(nil, first, t1, NewID(), NewID(), NewID(), t1)
	if err != nil {
		t.Fatal(err)
	}
	current := *c.Opened
	if *current.State.Finish != "без отделки" {
		t.Errorf("finish not trimmed: %q", *current.State.Finish)
	}

	// Unknown fields keep previous values: no change at all.
	c, err = ApplyObservation(&current, ObservedState{Price: Value(rub(15_900_000))}, t1.Add(time.Hour), NewID(), current.ListingID, NewID(), t1)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != ChangeConfirmed {
		t.Fatalf("unknown fields must carry values forward, got %s", c.Kind)
	}
	current = *c.Confirmed

	// Absent discount clears it: that is a change.
	c, err = ApplyObservation(&current, ObservedState{PriceBeforeDiscount: Absent[Money]()}, t1.Add(2*time.Hour), NewID(), current.ListingID, NewID(), t1)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != ChangeChanged || c.Opened.State.PriceBeforeDiscount != nil || *c.Opened.State.Rooms != 2 {
		t.Fatalf("absent discount: %+v", c.Opened)
	}
}

func TestApplyObservationRejects(t *testing.T) {
	t1 := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	c, err := ApplyObservation(nil, observedLot145(15_900_000), t1, NewID(), NewID(), NewID(), t1)
	if err != nil {
		t.Fatal(err)
	}
	current := *c.Opened
	current.LastObservedAt = t1.Add(48 * time.Hour)

	_, err = ApplyObservation(&current, observedLot145(16_000_000), t1.Add(-time.Hour), NewID(), current.ListingID, NewID(), t1)
	assertRuleCode(t, err, CodeObservationOutOfOrdr)

	_, err = ApplyObservation(&current, observedLot145(16_000_000), t1.Add(24*time.Hour), NewID(), current.ListingID, NewID(), t1)
	assertRuleCode(t, err, CodeHistoryConflict)

	invalid := observedLot145(15_900_000)
	invalid.PriceBeforeDiscount = Value(rub(15_000_000))
	_, err = ApplyObservation(&current, invalid, t1.Add(72*time.Hour), NewID(), current.ListingID, NewID(), t1)
	assertRuleCode(t, err, CodeDiscountNotGreater)

	_, err = ApplyObservation(nil, ObservedState{Status: Absent[LotStatus]()}, t1, NewID(), NewID(), NewID(), t1)
	assertRuleCode(t, err, CodeInvalidValue)
}

func TestNewIngestionBatch(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	header := func(mutate func(*BatchHeader)) BatchHeader {
		h := BatchHeader{DataSourceID: NewID(), OriginKind: OriginManualEntry, ObservedAt: now.Add(-time.Hour)}
		if mutate != nil {
			mutate(&h)
		}
		return h
	}
	b, err := NewIngestionBatch(NewID(), header(nil), now)
	if err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}
	// Manual entry defaults; a partial batch is the only supported completeness.
	if b.ObservedAtBasis != BasisEntryTime || b.Completeness != CompletenessPartial {
		t.Errorf("defaults = %q/%q, want entry_time/partial", b.ObservedAtBasis, b.Completeness)
	}
	if !b.ReceivedAt.Equal(now) || b.ConfirmedAt != nil {
		t.Errorf("received/confirmed = %v/%v", b.ReceivedAt, b.ConfirmedAt)
	}

	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) { h.ObservedAt = now.Add(time.Hour) }), now)
	assertRuleCode(t, err, CodeObservationInFuture)
	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) { h.OriginKind = "automated_collection" }), now)
	assertRuleCode(t, err, CodeInvalidValue)
	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) { h.DataSourceID = ID{} }), now)
	assertRuleCode(t, err, CodeRequired)

	// A document import must state what the moment of its data is based on.
	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) { h.OriginKind = OriginDocumentImport }), now)
	assertRuleCode(t, err, CodeRequired)
	doc, err := NewIngestionBatch(NewID(), header(func(h *BatchHeader) {
		h.OriginKind, h.ObservedAtBasis = OriginDocumentImport, BasisDocumentDate
	}), now)
	if err != nil || doc.ObservedAtBasis != BasisDocumentDate {
		t.Fatalf("document import with a basis: %+v, %v", doc, err)
	}
	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) { h.ObservedAtBasis = "guessed" }), now)
	assertRuleCode(t, err, CodeInvalidValue)

	// complete requires delisting by absence and is not supported yet.
	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) { h.Completeness = CompletenessComplete }), now)
	assertRuleCode(t, err, CodeInvalidValue)

	// Scope and confirmation.
	scoped, err := NewIngestionBatch(NewID(), header(func(h *BatchHeader) {
		confirmed := now.Add(-time.Minute)
		h.Scope = BatchScope{LotTypes: []LotType{LotTypeApartment}}
		h.ConfirmedAt = &confirmed
	}), now)
	if err != nil || scoped.ConfirmedAt == nil || !scoped.ConfirmedAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("scoped batch: %+v, %v", scoped, err)
	}
	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) {
		h.Scope = BatchScope{LotTypes: []LotType{LotTypeApartment, LotTypeApartment}}
	}), now)
	assertRuleCode(t, err, CodeInvalidValue)
	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) { h.Scope = BatchScope{LotTypes: []LotType{"villa"}} }), now)
	assertRuleCode(t, err, CodeInvalidValue)
	_, err = NewIngestionBatch(NewID(), header(func(h *BatchHeader) {
		confirmed := now.Add(time.Hour)
		h.ConfirmedAt = &confirmed
	}), now)
	assertRuleCode(t, err, CodeObservationInFuture)
}
