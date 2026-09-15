package domain

import (
	"strings"
	"unicode/utf8"
)

// LotStatus is the sales status stated by the source (Q-CORE-07). It is never
// inferred from a lot's absence. The set is temporary (A-20).
type LotStatus string

// Lot statuses.
const (
	LotStatusAvailable LotStatus = "available"
	LotStatusReserved  LotStatus = "reserved"
	LotStatusSold      LotStatus = "sold"
	LotStatusUnknown   LotStatus = "unknown"
)

// LotStatuses lists the supported statuses in display order.
var LotStatuses = []LotStatus{LotStatusAvailable, LotStatusReserved, LotStatusSold, LotStatusUnknown}

// Valid reports whether the status is known.
func (s LotStatus) Valid() bool {
	for _, v := range LotStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// Presence tells whether the source shows the lot. It is not a sales status.
type Presence string

// Presence values.
const (
	PresenceListed   Presence = "listed"
	PresenceDelisted Presence = "delisted"
)

// Valid reports whether the presence value is known.
func (p Presence) Valid() bool {
	return p == PresenceListed || p == PresenceDelisted
}

const maxFinishLength = 200

// LotState is the versioned state of a lot in one source. Nil pointers mean
// "no value". Price is the current offer price at the moment of observation;
// PriceBeforeDiscount is the base price before an explicitly stated discount
// (Q-CORE-15). Discount size and price per m² are calculated, not stored.
type LotState struct {
	Presence            Presence
	Status              LotStatus
	Rooms               *int
	AreaTotal           *Area
	Price               *Money
	PriceBeforeDiscount *Money
	// Finish is the finishing as stated by the source, free text until the
	// finishing dictionary is defined (Q-CORE-08, A-21).
	Finish *string
}

// Validate checks the state invariants.
func (s LotState) Validate() error {
	if !s.Presence.Valid() {
		return ruleErr(CodeInvalidValue, "presence", "unknown presence "+string(s.Presence))
	}
	if !s.Status.Valid() {
		return ruleErr(CodeInvalidValue, "status", "unknown status "+string(s.Status))
	}
	if s.Rooms != nil && *s.Rooms < 0 {
		return ruleErr(CodeMustNotBeNegative, "rooms", "rooms must not be negative")
	}
	if s.Rooms != nil && *s.Rooms > 32767 {
		return ruleErr(CodeOutOfRange, "rooms", "rooms is out of range")
	}
	if s.AreaTotal != nil && *s.AreaTotal <= 0 {
		return ruleErr(CodeMustBePositive, "area_total", "total area must be positive")
	}
	if s.Price != nil && *s.Price <= 0 {
		return ruleErr(CodeMustBePositive, "price", "price must be positive")
	}
	if s.PriceBeforeDiscount != nil {
		if *s.PriceBeforeDiscount <= 0 {
			return ruleErr(CodeMustBePositive, "price_before_discount", "price before discount must be positive")
		}
		if s.Price == nil {
			return ruleErr(CodeDiscountWithoutPrice, "price_before_discount", "price before discount requires a price")
		}
		if *s.PriceBeforeDiscount <= *s.Price {
			return ruleErr(CodeDiscountNotGreater, "price_before_discount", "price before discount must be greater than price")
		}
	}
	if s.Finish != nil && !utf8.ValidString(*s.Finish) {
		return ruleErr(CodeInvalidValue, "finish", "finish is not valid UTF-8")
	}
	if s.Finish != nil && utf8.RuneCountInString(*s.Finish) > maxFinishLength {
		return ruleErr(CodeTooLong, "finish", "finish is too long")
	}
	return nil
}

// Equal reports whether two states have the same versioned values.
func (s LotState) Equal(o LotState) bool {
	return s.Presence == o.Presence &&
		s.Status == o.Status &&
		equalPtr(s.Rooms, o.Rooms) &&
		equalPtr(s.AreaTotal, o.AreaTotal) &&
		equalPtr(s.Price, o.Price) &&
		equalPtr(s.PriceBeforeDiscount, o.PriceBeforeDiscount) &&
		equalPtr(s.Finish, o.Finish)
}

// PricePerSqm returns price / total area, if both are known.
func (s LotState) PricePerSqm() *Money {
	if s.Price == nil || s.AreaTotal == nil {
		return nil
	}
	v, ok := PricePerSqm(*s.Price, *s.AreaTotal)
	if !ok {
		return nil
	}
	return &v
}

// Discount returns the discount amount when the price before discount is known.
func (s LotState) Discount() *Money {
	if s.Price == nil || s.PriceBeforeDiscount == nil {
		return nil
	}
	d := *s.PriceBeforeDiscount - *s.Price
	return &d
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// FieldState distinguishes a known value, an unknown value and an explicitly
// absent value in an observation (ADR-0007, core-domain-model.md 8.5).
type FieldState uint8

// Field states. The zero value is FieldUnknown.
const (
	FieldUnknown FieldState = iota
	FieldValue
	FieldAbsent
)

// Field is an observed value with its state.
type Field[T comparable] struct {
	State FieldState
	Value T
}

// Value returns a field with a known value.
func Value[T comparable](v T) Field[T] { return Field[T]{State: FieldValue, Value: v} }

// Absent returns a field the source explicitly shows as absent.
func Absent[T comparable]() Field[T] { return Field[T]{State: FieldAbsent} }

// Unknown returns a field the source does not report.
func Unknown[T comparable]() Field[T] { return Field[T]{} }

// resolve applies the field to the previous value: unknown keeps it, absent
// clears it, a value replaces it.
func (f Field[T]) resolve(prev *T) *T {
	switch f.State {
	case FieldValue:
		v := f.Value
		return &v
	case FieldAbsent:
		return nil
	default:
		if prev == nil {
			return nil
		}
		v := *prev
		return &v
	}
}

// ObservedState is the lot state reported by one observation.
type ObservedState struct {
	// Presence defaults to listed when empty.
	Presence            Presence
	Status              Field[LotStatus]
	Rooms               Field[int]
	AreaTotal           Field[Area]
	Price               Field[Money]
	PriceBeforeDiscount Field[Money]
	Finish              Field[string]
}

// Resolve builds the resulting state from the previous state (nil when there
// is none) and validates it.
func (o ObservedState) Resolve(prev *LotState) (LotState, error) {
	var base LotState
	if prev != nil {
		base = *prev
	}
	presence := o.Presence
	if presence == "" {
		presence = PresenceListed
	}
	if o.Status.State == FieldAbsent {
		return LotState{}, ruleErr(CodeInvalidValue, "status", "status cannot be absent; use unknown")
	}
	status := o.Status.resolve(nilIfEmpty(base.Status))
	st := LotState{
		Presence:            presence,
		Status:              LotStatusUnknown,
		Rooms:               o.Rooms.resolve(base.Rooms),
		AreaTotal:           o.AreaTotal.resolve(base.AreaTotal),
		Price:               o.Price.resolve(base.Price),
		PriceBeforeDiscount: o.PriceBeforeDiscount.resolve(base.PriceBeforeDiscount),
		Finish:              normalizeFinish(o.Finish.resolve(base.Finish)),
	}
	if status != nil {
		st.Status = *status
	}
	if err := st.Validate(); err != nil {
		return LotState{}, err
	}
	return st, nil
}

func nilIfEmpty(s LotStatus) *LotStatus {
	if s == "" {
		return nil
	}
	return &s
}

func normalizeFinish(f *string) *string {
	if f == nil {
		return nil
	}
	v := strings.TrimSpace(*f)
	if v == "" {
		return nil
	}
	return &v
}
