package domain

import (
	"sort"
	"time"
)

// Version is an interval [ValidFrom, ValidTo) during which the state of a lot
// in one source did not change (ADR-0007). LastObservedAt is the latest
// observation that confirmed the values; between LastObservedAt and ValidTo
// the values are carried forward but not confirmed.
type Version struct {
	ID             ID
	ListingID      ID
	ValidFrom      time.Time
	ValidTo        *time.Time // nil for the current version
	LastObservedAt time.Time
	State          LotState
	BatchID        ID
	CreatedAt      time.Time
}

// IsCurrent reports whether the version is open-ended.
func (v Version) IsCurrent() bool { return v.ValidTo == nil }

// Contains reports whether moment t belongs to the version interval.
func (v Version) Contains(t time.Time) bool {
	return !t.Before(v.ValidFrom) && (v.ValidTo == nil || t.Before(*v.ValidTo))
}

// ChangeKind describes how an observation changed the history of a listing.
type ChangeKind string

// Change kinds.
const (
	ChangeCreated   ChangeKind = "created"   // first version of the listing
	ChangeChanged   ChangeKind = "changed"   // the current version was closed and a new one opened
	ChangeConfirmed ChangeKind = "confirmed" // same values: no new version, last observation moved
)

// Change is the result of applying an observation to a listing history.
type Change struct {
	Kind ChangeKind
	// Closed is the previous current version with ValidTo set (ChangeChanged).
	Closed *Version
	// Opened is the new current version (ChangeCreated, ChangeChanged).
	Opened *Version
	// Confirmed is the current version with an updated LastObservedAt (ChangeConfirmed).
	Confirmed *Version
}

// ApplyObservation applies an observation made at t to the current version of
// a listing (nil when the listing has no history yet).
//
// Core MVP rules (core-mvp.md): a new version is created only when versioned
// values change; an identical observation only moves LastObservedAt; an
// observation earlier than the current version is rejected (inserting into
// the past is deferred, Q-CORE-27); a different value at or before the last
// confirmation of the current version is a history conflict.
func ApplyObservation(current *Version, obs ObservedState, t time.Time, newVersionID, listingID, batchID ID, now time.Time) (Change, error) {
	if current == nil {
		st, err := obs.Resolve(nil)
		if err != nil {
			return Change{}, err
		}
		v := Version{ID: newVersionID, ListingID: listingID, ValidFrom: t, LastObservedAt: t, State: st, BatchID: batchID, CreatedAt: now}
		return Change{Kind: ChangeCreated, Opened: &v}, nil
	}
	if t.Before(current.ValidFrom) {
		return Change{}, ruleErr(CodeObservationOutOfOrdr, "observed_at", "observation is earlier than the current state of the lot")
	}
	st, err := obs.Resolve(&current.State)
	if err != nil {
		return Change{}, err
	}
	if st.Equal(current.State) {
		confirmed := *current
		if t.After(confirmed.LastObservedAt) {
			confirmed.LastObservedAt = t
		}
		return Change{Kind: ChangeConfirmed, Confirmed: &confirmed}, nil
	}
	if !t.After(current.LastObservedAt) {
		return Change{}, ruleErr(CodeHistoryConflict, "observed_at", "different values at or before the last confirmation of the current state")
	}
	closed := *current
	closedAt := t
	closed.ValidTo = &closedAt
	opened := Version{ID: newVersionID, ListingID: listingID, ValidFrom: t, LastObservedAt: t, State: st, BatchID: batchID, CreatedAt: now}
	return Change{Kind: ChangeChanged, Closed: &closed, Opened: &opened}, nil
}

// StateAt returns the version valid at moment t.
func StateAt(versions []Version, t time.Time) (Version, bool) {
	for _, v := range versions {
		if v.Contains(t) {
			return v, true
		}
	}
	return Version{}, false
}

// SortVersions orders versions by ValidFrom ascending.
func SortVersions(versions []Version) {
	sort.Slice(versions, func(i, j int) bool { return versions[i].ValidFrom.Before(versions[j].ValidFrom) })
}

// OriginKind is how a batch of observations reached Core.
type OriginKind string

// Origin kinds supported by the MVP. automated_collection is future scope.
const (
	OriginManualEntry    OriginKind = "manual_entry"
	OriginDocumentImport OriginKind = "document_import"
)

// Valid reports whether the origin kind is known.
func (k OriginKind) Valid() bool {
	return k == OriginManualEntry || k == OriginDocumentImport
}

// IngestionBatch is the header of one portion of confirmed data accepted by
// Core: provenance of the versions it creates (ADR-0007).
type IngestionBatch struct {
	ID           ID
	DataSourceID ID
	OriginKind   OriginKind
	OriginRef    *ID // import or entry id in the sending module; idempotency key
	ObservedAt   time.Time
	ReceivedAt   time.Time
}

// maxClockSkew tolerates small clock differences between the sender and Core.
const maxClockSkew = time.Minute

// NewIngestionBatch validates and creates a batch header.
func NewIngestionBatch(id, dataSourceID ID, kind OriginKind, originRef *ID, observedAt, now time.Time) (IngestionBatch, error) {
	if dataSourceID.IsZero() {
		return IngestionBatch{}, ruleErr(CodeRequired, "data_source", "data source is required")
	}
	if !kind.Valid() {
		return IngestionBatch{}, ruleErr(CodeInvalidValue, "origin_kind", "unknown origin kind "+string(kind))
	}
	if observedAt.IsZero() {
		return IngestionBatch{}, ruleErr(CodeRequired, "observed_at", "observation time is required")
	}
	if observedAt.After(now.Add(maxClockSkew)) {
		return IngestionBatch{}, ruleErr(CodeObservationInFuture, "observed_at", "observation time is in the future")
	}
	return IngestionBatch{ID: id, DataSourceID: dataSourceID, OriginKind: kind, OriginRef: originRef, ObservedAt: observedAt, ReceivedAt: now}, nil
}

// NormalizeTime converts t to UTC with PostgreSQL timestamp precision.
func NormalizeTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}
