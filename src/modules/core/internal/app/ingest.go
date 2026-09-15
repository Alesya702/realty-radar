package app

import (
	"context"
	"fmt"
	"time"

	"realtyradar/src/modules/core/internal/domain"
)

// ObservationBatch is the Core MVP ingestion contract (core-mvp.md): confirmed
// normalized observations of lots of one project from one data source at one
// moment. It is used by manual entry now and by PDF import next.
//
// Deliberately deferred from the target contract (core-domain-model.md 9.4):
// observed_at basis, scope and completeness (no delisting by absence),
// resolve_by_source_key targets, placement updates of existing lots, and
// batch correction.
type ObservationBatch struct {
	DataSourceID domain.ID
	OriginKind   domain.OriginKind
	// OriginRef identifies the confirmed import or entry in the sending module.
	// A repeated batch with the same OriginKind and OriginRef is not applied again.
	OriginRef  *domain.ID
	ObservedAt time.Time
	ProjectID  domain.ID
	// Buildings and Sections declare the structure the items refer to: an
	// existing object or a new one confirmed by the user.
	Buildings []BuildingDecision
	Sections  []SectionDecision
	Items     []LotObservation
}

// BuildingDecision refers to an existing building of the project or creates one.
type BuildingDecision struct {
	Ref        string
	ExistingID *domain.ID
	NewName    string
}

// SectionDecision refers to an existing section or creates one in a building
// declared in the same batch.
type SectionDecision struct {
	Ref         string
	ExistingID  *domain.ID
	BuildingRef string
	NewName     string
}

// LotTarget is the confirmed matching decision: an existing lot or a new lot
// with the given placement. Exactly one of the fields is set.
type LotTarget struct {
	ExistingLotID *domain.ID
	NewLot        *NewLotPlacement
}

// NewLotPlacement is the placement of a new lot. BuildingRef and SectionRef
// refer to decisions of the batch; empty means the level is absent.
type NewLotPlacement struct {
	BuildingRef string
	SectionRef  string
	Type        domain.LotType
	Number      string
	Floor       *int
}

// LotObservation is the confirmed state of one lot.
type LotObservation struct {
	Ref         string
	Target      LotTarget
	SourceKey   *string
	SourceURL   *string
	PublishedAt *time.Time
	State       domain.ObservedState
}

// ItemResult reports what happened to one item.
type ItemResult struct {
	Ref       string
	LotID     domain.ID
	ListingID domain.ID
	LotIsNew  bool
	Change    domain.ChangeKind
	VersionID domain.ID
}

// BatchResult reports the outcome of a batch.
type BatchResult struct {
	BatchID domain.ID
	// AlreadyApplied is true when a batch with the same origin was applied
	// before; nothing was changed and Items is empty.
	AlreadyApplied bool
	Items          []ItemResult
}

// ItemError locates a rule violation inside a batch.
type ItemError struct {
	Ref string
	Err error
}

func (e *ItemError) Error() string { return fmt.Sprintf("item %q: %v", e.Ref, e.Err) }
func (e *ItemError) Unwrap() error { return e.Err }

// IngestObservationBatch applies a batch atomically: either all items are
// applied or none.
func (s *Service) IngestObservationBatch(ctx context.Context, in ObservationBatch) (BatchResult, error) {
	now := s.clock()
	observedAt := domain.NormalizeTime(in.ObservedAt)
	batch, err := domain.NewIngestionBatch(s.newID(), in.DataSourceID, in.OriginKind, in.OriginRef, observedAt, now)
	if err != nil {
		return BatchResult{}, err
	}

	var result BatchResult
	err = s.repo.InTx(ctx, func(st Store) error {
		if in.OriginRef != nil {
			prev, err := st.FindBatchByOrigin(ctx, in.OriginKind, *in.OriginRef)
			if err != nil {
				return fmt.Errorf("ingest: find batch: %w", err)
			}
			if prev != nil {
				result = BatchResult{BatchID: prev.ID, AlreadyApplied: true}
				return nil
			}
		}
		src, err := st.GetDataSource(ctx, in.DataSourceID)
		if err != nil {
			return fmt.Errorf("ingest: data source %s: %w", in.DataSourceID, err)
		}
		if !src.IsActive {
			return &domain.RuleError{Code: domain.CodeSourceInactive, Field: "data_source", Detail: "data source " + src.Code + " is inactive"}
		}
		if err := st.LockProject(ctx, in.ProjectID); err != nil {
			return fmt.Errorf("ingest: project %s: %w", in.ProjectID, err)
		}
		if err := st.InsertBatch(ctx, batch); err != nil {
			return fmt.Errorf("ingest: insert batch: %w", err)
		}

		ing := &ingestion{svc: s, st: st, batch: batch, projectID: in.ProjectID, now: now,
			buildings: map[string]domain.Building{}, sections: map[string]domain.Section{}, seenLots: map[domain.ID]string{}}
		if err := ing.resolveStructure(ctx, in); err != nil {
			return err
		}
		result = BatchResult{BatchID: batch.ID, Items: make([]ItemResult, 0, len(in.Items))}
		for _, item := range in.Items {
			r, err := ing.apply(ctx, item)
			if err != nil {
				return &ItemError{Ref: item.Ref, Err: err}
			}
			result.Items = append(result.Items, r)
		}
		return nil
	})
	if err != nil {
		return BatchResult{}, err
	}
	return result, nil
}

type ingestion struct {
	svc       *Service
	st        Store
	batch     domain.IngestionBatch
	projectID domain.ID
	now       time.Time
	buildings map[string]domain.Building
	sections  map[string]domain.Section
	seenLots  map[domain.ID]string
}

func (ing *ingestion) resolveStructure(ctx context.Context, in ObservationBatch) error {
	for _, d := range in.Buildings {
		if d.Ref == "" || ing.hasRef(d.Ref) {
			return &ItemError{Ref: d.Ref, Err: &domain.RuleError{Code: domain.CodeDuplicateInBatch, Field: "ref", Detail: "building ref must be unique and non-empty"}}
		}
		var b domain.Building
		var err error
		switch {
		case d.ExistingID != nil && d.NewName == "":
			b, err = ing.st.GetBuilding(ctx, *d.ExistingID)
			if err == nil && b.ProjectID != ing.projectID {
				err = &domain.RuleError{Code: domain.CodeWrongProject, Field: "building", Detail: "building belongs to another project"}
			}
		case d.ExistingID == nil && d.NewName != "":
			b, err = addBuilding(ctx, ing.st, ing.svc.newID(), ing.projectID, d.NewName, ing.now)
		default:
			err = &domain.RuleError{Code: domain.CodeInvalidTarget, Field: "building", Detail: "set either an existing building or a new name"}
		}
		if err != nil {
			return &ItemError{Ref: d.Ref, Err: err}
		}
		ing.buildings[d.Ref] = b
	}
	for _, d := range in.Sections {
		if d.Ref == "" || ing.hasRef(d.Ref) {
			return &ItemError{Ref: d.Ref, Err: &domain.RuleError{Code: domain.CodeDuplicateInBatch, Field: "ref", Detail: "section ref must be unique and non-empty"}}
		}
		var sec domain.Section
		var err error
		switch {
		case d.ExistingID != nil && d.NewName == "":
			sec, err = ing.st.GetSection(ctx, *d.ExistingID)
			if err == nil {
				var b domain.Building
				b, err = ing.st.GetBuilding(ctx, sec.BuildingID)
				if err == nil && b.ProjectID != ing.projectID {
					err = &domain.RuleError{Code: domain.CodeWrongProject, Field: "section", Detail: "section belongs to another project"}
				}
			}
		case d.ExistingID == nil && d.NewName != "":
			b, ok := ing.buildings[d.BuildingRef]
			if !ok {
				err = &domain.RuleError{Code: domain.CodeUnknownRef, Field: "building", Detail: "unknown building ref " + d.BuildingRef}
				break
			}
			sec, err = addSection(ctx, ing.st, ing.svc.newID(), b, d.NewName, ing.now)
		default:
			err = &domain.RuleError{Code: domain.CodeInvalidTarget, Field: "section", Detail: "set either an existing section or a new name"}
		}
		if err != nil {
			return &ItemError{Ref: d.Ref, Err: err}
		}
		ing.sections[d.Ref] = sec
	}
	return nil
}

func (ing *ingestion) hasRef(ref string) bool {
	_, b := ing.buildings[ref]
	_, s := ing.sections[ref]
	return b || s
}

func (ing *ingestion) apply(ctx context.Context, item LotObservation) (ItemResult, error) {
	lot, isNew, err := ing.resolveLot(ctx, item.Target)
	if err != nil {
		return ItemResult{}, err
	}
	if prevRef, dup := ing.seenLots[lot.ID]; dup {
		return ItemResult{}, &domain.RuleError{Code: domain.CodeDuplicateInBatch, Field: "lot", Detail: "the lot is already observed by item " + prevRef}
	}
	ing.seenLots[lot.ID] = item.Ref

	listing, err := ing.resolveListing(ctx, lot, item)
	if err != nil {
		return ItemResult{}, err
	}

	current, err := ing.st.LockCurrentVersion(ctx, listing.ID)
	if err != nil {
		return ItemResult{}, fmt.Errorf("lock current version of listing %s: %w", listing.ID, err)
	}
	change, err := domain.ApplyObservation(current, item.State, ing.batch.ObservedAt, ing.svc.newID(), listing.ID, ing.batch.ID, ing.now)
	if err != nil {
		return ItemResult{}, err
	}
	res := ItemResult{Ref: item.Ref, LotID: lot.ID, ListingID: listing.ID, LotIsNew: isNew, Change: change.Kind}
	switch change.Kind {
	case domain.ChangeCreated:
		err = ing.st.InsertVersion(ctx, *change.Opened)
		res.VersionID = change.Opened.ID
	case domain.ChangeChanged:
		if err = ing.st.CloseVersion(ctx, change.Closed.ID, *change.Closed.ValidTo); err == nil {
			err = ing.st.InsertVersion(ctx, *change.Opened)
		}
		res.VersionID = change.Opened.ID
	case domain.ChangeConfirmed:
		err = ing.st.UpdateVersionLastObserved(ctx, change.Confirmed.ID, change.Confirmed.LastObservedAt)
		res.VersionID = change.Confirmed.ID
	}
	if err != nil {
		return ItemResult{}, fmt.Errorf("write history of listing %s: %w", listing.ID, err)
	}

	if item.State.Presence != domain.PresenceDelisted {
		listing.Seen(ing.batch.ObservedAt)
	}
	if err := ing.st.UpdateListing(ctx, listing); err != nil {
		return ItemResult{}, fmt.Errorf("update listing %s: %w", listing.ID, err)
	}
	if lot.PrimaryListingID == nil {
		if err := ing.st.SetPrimaryListing(ctx, lot.ID, listing.ID); err != nil {
			return ItemResult{}, fmt.Errorf("set primary listing of lot %s: %w", lot.ID, err)
		}
	}
	return res, nil
}

func (ing *ingestion) resolveLot(ctx context.Context, target LotTarget) (domain.Lot, bool, error) {
	switch {
	case target.ExistingLotID != nil && target.NewLot == nil:
		lot, err := ing.st.GetLot(ctx, *target.ExistingLotID)
		if err != nil {
			return domain.Lot{}, false, fmt.Errorf("lot %s: %w", target.ExistingLotID, err)
		}
		if lot.Placement.ProjectID != ing.projectID {
			return domain.Lot{}, false, &domain.RuleError{Code: domain.CodeWrongProject, Field: "lot", Detail: "the lot belongs to another project"}
		}
		return lot, false, nil
	case target.ExistingLotID == nil && target.NewLot != nil:
		lot, err := ing.createLot(ctx, *target.NewLot)
		return lot, true, err
	default:
		return domain.Lot{}, false, &domain.RuleError{Code: domain.CodeInvalidTarget, Field: "lot", Detail: "set either an existing lot or a new lot placement"}
	}
}

func (ing *ingestion) createLot(ctx context.Context, np NewLotPlacement) (domain.Lot, error) {
	p := domain.Placement{ProjectID: ing.projectID, Type: np.Type, Number: np.Number, Floor: np.Floor}
	var building *domain.Building
	var section *domain.Section
	if np.BuildingRef != "" {
		b, ok := ing.buildings[np.BuildingRef]
		if !ok {
			return domain.Lot{}, &domain.RuleError{Code: domain.CodeUnknownRef, Field: "building", Detail: "unknown building ref " + np.BuildingRef}
		}
		building, p.BuildingID = &b, &b.ID
	}
	if np.SectionRef != "" {
		sec, ok := ing.sections[np.SectionRef]
		if !ok {
			return domain.Lot{}, &domain.RuleError{Code: domain.CodeUnknownRef, Field: "section", Detail: "unknown section ref " + np.SectionRef}
		}
		section, p.SectionID = &sec, &sec.ID
	}
	lot, err := domain.NewLot(ing.svc.newID(), p, building, section, ing.now)
	if err != nil {
		return domain.Lot{}, err
	}
	// No fuzzy matching: only an exact duplicate of the placement is refused,
	// so that a confirmed "new lot" decision cannot silently duplicate a lot.
	dup, err := ing.st.FindLotByPlacement(ctx, lot.Placement)
	if err != nil {
		return domain.Lot{}, fmt.Errorf("check lot placement: %w", err)
	}
	if dup != nil {
		return domain.Lot{}, &domain.RuleError{Code: domain.CodeLotAlreadyExists, Field: "number", Detail: "a lot with the same placement already exists: " + dup.ID.String()}
	}
	if err := ing.st.InsertLot(ctx, lot); err != nil {
		return domain.Lot{}, fmt.Errorf("insert lot: %w", err)
	}
	return lot, nil
}

func (ing *ingestion) resolveListing(ctx context.Context, lot domain.Lot, item LotObservation) (domain.Listing, error) {
	key, err := domain.NormalizeSourceKey(item.SourceKey)
	if err != nil {
		return domain.Listing{}, err
	}
	sourceURL, err := domain.NormalizeSourceURL(item.SourceURL)
	if err != nil {
		return domain.Listing{}, err
	}
	if key != nil {
		owner, err := ing.st.FindListingBySourceKey(ctx, ing.batch.DataSourceID, *key)
		if err != nil {
			return domain.Listing{}, fmt.Errorf("find listing by source key: %w", err)
		}
		if owner != nil && owner.LotID != lot.ID {
			return domain.Listing{}, &domain.RuleError{Code: domain.CodeSourceKeyConflict, Field: "source_key", Detail: "the source key belongs to another lot"}
		}
	}
	existing, err := ing.st.FindListing(ctx, lot.ID, ing.batch.DataSourceID)
	if err != nil {
		return domain.Listing{}, fmt.Errorf("find listing: %w", err)
	}
	var listing domain.Listing
	if existing == nil {
		listing = domain.NewListing(ing.svc.newID(), lot.ID, ing.batch.DataSourceID, ing.batch.ObservedAt, ing.now)
		listing.SourceKey = key
		if err := ing.st.InsertListing(ctx, listing); err != nil {
			return domain.Listing{}, fmt.Errorf("insert listing: %w", err)
		}
	} else {
		listing = *existing
		if key != nil {
			if listing.SourceKey != nil && *listing.SourceKey != *key {
				// Key history is deferred (core-mvp.md).
				return domain.Listing{}, &domain.RuleError{Code: domain.CodeSourceKeyConflict, Field: "source_key", Detail: "the listing already has another source key"}
			}
			listing.SourceKey = key
		}
	}
	if sourceURL != nil {
		listing.SourceURL = sourceURL
	}
	if item.PublishedAt != nil {
		d := domain.DateOf(*item.PublishedAt)
		listing.PublishedAt = &d
	}
	return listing, nil
}
