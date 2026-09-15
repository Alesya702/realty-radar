package domain

import "time"

// RelationType is the kind of a directed relation "related project is X for
// the subject project". Only the kinds from ТЗ п. 1 exist.
type RelationType string

// Relation types.
const (
	RelationDirectCompetitor   RelationType = "direct_competitor"
	RelationIndirectCompetitor RelationType = "indirect_competitor"
	RelationReference          RelationType = "reference"
)

// RelationTypes lists the relation types in display order.
var RelationTypes = []RelationType{RelationDirectCompetitor, RelationIndirectCompetitor, RelationReference}

// Valid reports whether the relation type is known.
func (t RelationType) Valid() bool {
	for _, v := range RelationTypes {
		if v == t {
			return true
		}
	}
	return false
}

const maxRationaleLength = 2000

// ProjectRelation is a contextual, directed relation Project → Project
// (core-domain-model.md, 4.3). Competition is not a property of a project.
type ProjectRelation struct {
	ID               ID
	SubjectProjectID ID
	RelatedProjectID ID
	Type             RelationType
	Rationale        string
	ValidFrom        time.Time  // date
	ValidTo          *time.Time // date, inclusive; nil while the relation holds
	CreatedAt        time.Time
}

// NewProjectRelation validates a relation against the existing relations of
// the same subject project and creates it.
func NewProjectRelation(id, subject, related ID, typ RelationType, rationale string, validFrom time.Time, validTo *time.Time, existing []ProjectRelation, now time.Time) (ProjectRelation, error) {
	if subject.IsZero() || related.IsZero() {
		return ProjectRelation{}, ruleErr(CodeRequired, "project", "both projects are required")
	}
	if subject == related {
		return ProjectRelation{}, ruleErr(CodeSelfRelation, "related_project", "a project cannot be related to itself")
	}
	if !typ.Valid() {
		return ProjectRelation{}, ruleErr(CodeInvalidValue, "relation_type", "unknown relation type "+string(typ))
	}
	rationale, err := optionalText("rationale", rationale, maxRationaleLength)
	if err != nil {
		return ProjectRelation{}, err
	}
	if validFrom.IsZero() {
		return ProjectRelation{}, ruleErr(CodeRequired, "valid_from", "start date is required")
	}
	validFrom = DateOf(validFrom)
	if validTo != nil {
		to := DateOf(*validTo)
		validTo = &to
		if to.Before(validFrom) {
			return ProjectRelation{}, ruleErr(CodePeriodOrder, "valid_to", "end date is before start date")
		}
	}
	for _, e := range existing {
		if e.SubjectProjectID == subject && e.RelatedProjectID == related && e.Type == typ &&
			datePeriodsOverlap(&e.ValidFrom, e.ValidTo, &validFrom, validTo) {
			return ProjectRelation{}, ruleErr(CodePeriodOverlap, "relation_type", "the same relation already exists for this period")
		}
	}
	return ProjectRelation{
		ID: id, SubjectProjectID: subject, RelatedProjectID: related, Type: typ,
		Rationale: rationale, ValidFrom: validFrom, ValidTo: validTo, CreatedAt: now,
	}, nil
}

// ActiveOn reports whether the relation holds on the given date.
func (r ProjectRelation) ActiveOn(date time.Time) bool {
	d := DateOf(date)
	return !d.Before(r.ValidFrom) && (r.ValidTo == nil || !d.After(*r.ValidTo))
}

// PortfolioMembership states that a project belongs to the company portfolio
// during a period (core-domain-model.md, 4.2). Nil bounds are open.
type PortfolioMembership struct {
	ID        ID
	ProjectID ID
	ValidFrom *time.Time // date; nil = unknown / since the beginning
	ValidTo   *time.Time // date, inclusive; nil = still in the portfolio
	Note      string
	CreatedAt time.Time
}

// NewPortfolioMembership validates a membership against existing memberships
// of the project and creates it.
func NewPortfolioMembership(id, projectID ID, validFrom, validTo *time.Time, note string, existing []PortfolioMembership, now time.Time) (PortfolioMembership, error) {
	if projectID.IsZero() {
		return PortfolioMembership{}, ruleErr(CodeRequired, "project", "project is required")
	}
	note, err := optionalText("note", note, maxRationaleLength)
	if err != nil {
		return PortfolioMembership{}, err
	}
	if validFrom != nil {
		from := DateOf(*validFrom)
		validFrom = &from
	}
	if validTo != nil {
		to := DateOf(*validTo)
		validTo = &to
	}
	if validFrom != nil && validTo != nil && validTo.Before(*validFrom) {
		return PortfolioMembership{}, ruleErr(CodePeriodOrder, "valid_to", "end date is before start date")
	}
	for _, e := range existing {
		if e.ProjectID == projectID && datePeriodsOverlap(e.ValidFrom, e.ValidTo, validFrom, validTo) {
			return PortfolioMembership{}, ruleErr(CodePeriodOverlap, "valid_from", "the project is already in the portfolio for this period")
		}
	}
	return PortfolioMembership{ID: id, ProjectID: projectID, ValidFrom: validFrom, ValidTo: validTo, Note: note, CreatedAt: now}, nil
}

// ActiveOn reports whether the project is in the portfolio on the given date.
func (m PortfolioMembership) ActiveOn(date time.Time) bool {
	d := DateOf(date)
	return (m.ValidFrom == nil || !d.Before(*m.ValidFrom)) && (m.ValidTo == nil || !d.After(*m.ValidTo))
}

// DateOf returns the calendar date of t (in t's location) as UTC midnight.
func DateOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// datePeriodsOverlap reports whether two inclusive date periods overlap; nil
// bounds are open.
func datePeriodsOverlap(aFrom, aTo, bFrom, bTo *time.Time) bool {
	// a ends before b starts, or b ends before a starts.
	if aTo != nil && bFrom != nil && aTo.Before(*bFrom) {
		return false
	}
	if bTo != nil && aFrom != nil && bTo.Before(*aFrom) {
		return false
	}
	return true
}
