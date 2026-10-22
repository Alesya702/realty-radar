package domain

import (
	"math"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// LotType classifies a lot by purpose. The set follows ТЗ п. 6 and is
// temporary until Q-CORE-02 is answered (A-19).
type LotType string

// Lot types.
const (
	LotTypeApartment  LotType = "apartment"
	LotTypeCommercial LotType = "commercial"
	LotTypeStorage    LotType = "storage"
	LotTypeParking    LotType = "parking"
)

// LotTypes lists the supported lot types in display order.
var LotTypes = []LotType{LotTypeApartment, LotTypeCommercial, LotTypeStorage, LotTypeParking}

// Valid reports whether the lot type is known.
func (t LotType) Valid() bool {
	for _, v := range LotTypes {
		if v == t {
			return true
		}
	}
	return false
}

// Placement is where a lot is: project (required), building and section
// (optional), type, number (optional) and floor. Placement is not versioned:
// changing it is a correction, not a market event (core-domain-model.md, 6.1).
type Placement struct {
	ProjectID  ID
	BuildingID *ID
	SectionID  *ID
	Type       LotType
	// Number is the lot number stated by the source; empty when the source
	// states none (stored as NULL). A missing number is not an identity: lots
	// without a number are never matched by placement (see MatchPlacement).
	Number string
	Floor  *int
}

// Lot is the canonical object of sale.
type Lot struct {
	ID               ID
	Placement        Placement
	PrimaryListingID *ID
	CreatedAt        time.Time
}

// NewLot validates the placement against the (optional) building and section
// and creates a lot. building and section must be the objects referenced by
// the placement, or nil when the placement has no building or section.
func NewLot(id ID, p Placement, building *Building, section *Section, now time.Time) (Lot, error) {
	p, err := ValidatePlacement(p, building, section)
	if err != nil {
		return Lot{}, err
	}
	return Lot{ID: id, Placement: p, CreatedAt: now}, nil
}

// ValidatePlacement checks and normalizes a lot placement. Missing hierarchy
// levels are allowed; an inconsistent hierarchy is not.
func ValidatePlacement(p Placement, building *Building, section *Section) (Placement, error) {
	if p.ProjectID.IsZero() {
		return p, ruleErr(CodeRequired, "project", "project is required")
	}
	if !p.Type.Valid() {
		return p, ruleErr(CodeInvalidValue, "lot_type", "unknown lot type "+string(p.Type))
	}
	number, err := optionalText("number", p.Number, maxNumberLength)
	if err != nil {
		return p, err
	}
	p.Number = number
	if p.Floor != nil && (*p.Floor < math.MinInt16 || *p.Floor > math.MaxInt16) {
		return p, ruleErr(CodeOutOfRange, "floor", "floor is out of range")
	}
	if p.SectionID != nil && p.BuildingID == nil {
		return p, ruleErr(CodeSectionWithoutBldg, "section", "a section requires a building")
	}
	if p.BuildingID != nil {
		if building == nil || building.ID != *p.BuildingID {
			return p, ruleErr(CodePlacementMismatch, "building", "building does not match placement")
		}
		if building.ProjectID != p.ProjectID {
			return p, ruleErr(CodePlacementMismatch, "building", "building belongs to another project")
		}
	}
	if p.SectionID != nil {
		if section == nil || section.ID != *p.SectionID {
			return p, ruleErr(CodePlacementMismatch, "section", "section does not match placement")
		}
		if section.BuildingID != *p.BuildingID {
			return p, ruleErr(CodePlacementMismatch, "section", "section belongs to another building")
		}
	}
	return p, nil
}

// Listing is a lot in one data source (ADR-0006): at most one per lot and
// source. It keeps the source identity and first/last seen times.
type Listing struct {
	ID           ID
	LotID        ID
	DataSourceID ID
	SourceKey    *string
	SourceURL    *string
	PublishedAt  *time.Time
	FirstSeenAt  time.Time
	LastSeenAt   time.Time
	CreatedAt    time.Time
}

// NewListing creates a listing first observed at observedAt.
func NewListing(id, lotID, dataSourceID ID, observedAt, now time.Time) Listing {
	return Listing{
		ID:           id,
		LotID:        lotID,
		DataSourceID: dataSourceID,
		FirstSeenAt:  observedAt,
		LastSeenAt:   observedAt,
		CreatedAt:    now,
	}
}

// Seen extends first/last seen with an observation at t.
func (l *Listing) Seen(t time.Time) {
	if t.Before(l.FirstSeenAt) {
		l.FirstSeenAt = t
	}
	if t.After(l.LastSeenAt) {
		l.LastSeenAt = t
	}
}

// NormalizeSourceKey trims a source key; empty means "no key".
func NormalizeSourceKey(key *string) (*string, error) {
	if key == nil {
		return nil, nil
	}
	k := strings.TrimSpace(*key)
	if k == "" {
		return nil, nil
	}
	if !utf8.ValidString(k) {
		return nil, ruleErr(CodeInvalidValue, "source_key", "source key is not valid UTF-8")
	}
	if len(k) > maxNameLength {
		return nil, ruleErr(CodeTooLong, "source_key", "source key is too long")
	}
	return &k, nil
}

// NormalizeSourceURL validates an optional http(s) URL.
func NormalizeSourceURL(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, ruleErr(CodeInvalidValue, "source_url", "source URL must be an absolute http(s) URL")
	}
	if len(s) > 2000 {
		return nil, ruleErr(CodeTooLong, "source_url", "source URL is too long")
	}
	return &s, nil
}
