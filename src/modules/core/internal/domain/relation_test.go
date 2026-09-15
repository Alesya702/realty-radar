package domain

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestNewProjectRelation(t *testing.T) {
	a, b, c := NewID(), NewID(), NewID()
	now := time.Now()
	existing := []ProjectRelation{
		{ID: NewID(), SubjectProjectID: a, RelatedProjectID: b, Type: RelationDirectCompetitor, ValidFrom: date(2026, 1, 1), ValidTo: ptr(date(2026, 6, 30))},
	}

	tests := []struct {
		name     string
		subject  ID
		related  ID
		typ      RelationType
		from     time.Time
		to       *time.Time
		wantCode string
	}{
		{"new competitor", a, c, RelationDirectCompetitor, date(2026, 9, 1), nil, ""},
		{"same pair after previous period ended", a, b, RelationDirectCompetitor, date(2026, 7, 1), nil, ""},
		{"same pair, other type in same period", a, b, RelationReference, date(2026, 3, 1), nil, ""},
		{"opposite direction is a different relation", b, a, RelationDirectCompetitor, date(2026, 3, 1), nil, ""},
		{"overlapping same relation", a, b, RelationDirectCompetitor, date(2026, 6, 30), nil, CodePeriodOverlap},
		{"self relation", a, a, RelationReference, date(2026, 1, 1), nil, CodeSelfRelation},
		{"unknown type (no competitor flag or other kinds)", a, c, "partner", date(2026, 1, 1), nil, CodeInvalidValue},
		{"end before start", a, c, RelationReference, date(2026, 5, 1), ptr(date(2026, 4, 30)), CodePeriodOrder},
		{"missing start", a, c, RelationReference, time.Time{}, nil, CodeRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProjectRelation(NewID(), tt.subject, tt.related, tt.typ, "", tt.from, tt.to, existing, now)
			assertRuleCode(t, err, tt.wantCode)
		})
	}
}

func TestProjectRelationActiveOn(t *testing.T) {
	r := ProjectRelation{ValidFrom: date(2026, 1, 1), ValidTo: ptr(date(2026, 6, 30))}
	for _, tt := range []struct {
		at   time.Time
		want bool
	}{
		{date(2025, 12, 31), false},
		{date(2026, 1, 1), true},
		{time.Date(2026, 6, 30, 23, 59, 0, 0, time.UTC), true},
		{date(2026, 7, 1), false},
	} {
		if got := r.ActiveOn(tt.at); got != tt.want {
			t.Errorf("ActiveOn(%s) = %v, want %v", tt.at, got, tt.want)
		}
	}
}

func TestNewPortfolioMembership(t *testing.T) {
	p := NewID()
	now := time.Now()
	closed := PortfolioMembership{ID: NewID(), ProjectID: p, ValidFrom: ptr(date(2024, 1, 1)), ValidTo: ptr(date(2025, 12, 31))}

	if _, err := NewPortfolioMembership(NewID(), p, ptr(date(2026, 1, 1)), nil, "", []PortfolioMembership{closed}, now); err != nil {
		t.Errorf("membership after a closed one rejected: %v", err)
	}
	_, err := NewPortfolioMembership(NewID(), p, nil, nil, "", []PortfolioMembership{closed}, now)
	assertRuleCode(t, err, CodePeriodOverlap)
	_, err = NewPortfolioMembership(NewID(), p, ptr(date(2026, 2, 1)), ptr(date(2026, 1, 1)), "", nil, now)
	assertRuleCode(t, err, CodePeriodOrder)

	open := PortfolioMembership{ProjectID: p}
	if !open.ActiveOn(date(1990, 1, 1)) {
		t.Error("membership without bounds must be active on any date")
	}
	if closed.ActiveOn(date(2026, 1, 1)) || !closed.ActiveOn(date(2025, 12, 31)) {
		t.Error("closed membership bounds are wrong")
	}
}
