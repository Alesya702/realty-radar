package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"realtyradar/src/modules/core/internal/app"
	"realtyradar/src/modules/core/internal/domain"
)

// ---- lots ------------------------------------------------------------------------

const lotColumns = `l.id, l.project_id, l.building_id, l.section_id, l.lot_type, l.number, l.floor, l.primary_listing_id, l.created_at`

func scanLot(row pgx.Row, extra ...any) (domain.Lot, error) {
	var lot domain.Lot
	var id, project, building, section, primary pgtype.UUID
	var lotType string
	var number pgtype.Text
	var floor pgtype.Int2
	dest := append([]any{&id, &project, &building, &section, &lotType, &number, &floor, &primary, &lot.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return domain.Lot{}, err
	}
	lot.ID = idFrom(id)
	lot.Placement.ProjectID = idFrom(project)
	lot.Placement.BuildingID = idPtrFrom(building)
	lot.Placement.SectionID = idPtrFrom(section)
	lot.Placement.Type = domain.LotType(lotType)
	lot.Placement.Number = number.String // NULL: no number
	lot.Placement.Floor = intPtr(floor)
	lot.PrimaryListingID = idPtrFrom(primary)
	return lot, nil
}

func (s *store) InsertLot(ctx context.Context, lot domain.Lot) error {
	p := lot.Placement
	_, err := s.q.Exec(ctx, `
		INSERT INTO core.lots (id, project_id, building_id, section_id, lot_type, number, floor, primary_listing_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		uuidArg(lot.ID), uuidArg(p.ProjectID), nullUUIDArg(p.BuildingID), nullUUIDArg(p.SectionID),
		string(p.Type), optionalTextArg(p.Number), intArg(p.Floor), nullUUIDArg(lot.PrimaryListingID), lot.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert lot: %w", err)
	}
	return nil
}

func (s *store) GetLot(ctx context.Context, id domain.ID) (domain.Lot, error) {
	lot, err := scanLot(s.q.QueryRow(ctx, `SELECT `+lotColumns+` FROM core.lots l WHERE l.id = $1`, uuidArg(id)))
	if err != nil {
		return domain.Lot{}, notFound(err, "lot", id)
	}
	return lot, nil
}

func (s *store) ListLotsByNumber(ctx context.Context, projectID domain.ID, lotType domain.LotType, number string) ([]domain.Lot, error) {
	rows, err := s.q.Query(ctx, `
		SELECT `+lotColumns+` FROM core.lots l
		WHERE l.project_id = $1 AND l.lot_type = $2 AND l.number = $3
		ORDER BY l.id`,
		uuidArg(projectID), string(lotType), number)
	if err != nil {
		return nil, fmt.Errorf("list lots by number: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Lot, error) { return scanLot(row) })
}

func (s *store) SetPrimaryListing(ctx context.Context, lotID, listingID domain.ID) error {
	tag, err := s.q.Exec(ctx, `UPDATE core.lots SET primary_listing_id = $2, updated_at = now() WHERE id = $1`,
		uuidArg(lotID), uuidArg(listingID))
	if err != nil {
		return fmt.Errorf("set primary listing: %w", err)
	}
	return requireAffected(tag, "lot", lotID)
}

func (s *store) ListLotRows(ctx context.Context, projectID domain.ID) ([]app.LotRow, error) {
	rows, err := s.q.Query(ctx, `
		SELECT `+lotColumns+`, COALESCE(b.name, ''), COALESCE(s.name, ''), `+versionColumns+`
		FROM core.lots l
		LEFT JOIN core.buildings b ON b.id = l.building_id
		LEFT JOIN core.sections s ON s.id = l.section_id
		LEFT JOIN core.lot_listing_versions v ON v.listing_id = l.primary_listing_id AND v.valid_to IS NULL
		WHERE l.project_id = $1
		ORDER BY b.name NULLS FIRST, s.name NULLS FIRST, l.lot_type, length(l.number), l.number, l.id`,
		uuidArg(projectID))
	if err != nil {
		return nil, fmt.Errorf("list lots: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (app.LotRow, error) {
		var r app.LotRow
		var vs versionScan
		lot, err := scanLot(row, append([]any{&r.BuildingName, &r.SectionName}, vs.dest()...)...)
		if err != nil {
			return r, err
		}
		r.Lot = lot
		r.Current, err = vs.version()
		return r, err
	})
}

// ---- listings --------------------------------------------------------------------

const listingColumns = `li.id, li.lot_id, li.data_source_id, li.source_key, li.source_url, li.published_at, li.first_seen_at, li.last_seen_at, li.created_at`

func scanListing(row pgx.Row, extra ...any) (domain.Listing, error) {
	var l domain.Listing
	var id, lot, source pgtype.UUID
	var key, url pgtype.Text
	var published pgtype.Date
	dest := append([]any{&id, &lot, &source, &key, &url, &published, &l.FirstSeenAt, &l.LastSeenAt, &l.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return domain.Listing{}, err
	}
	l.ID, l.LotID, l.DataSourceID = idFrom(id), idFrom(lot), idFrom(source)
	l.SourceKey, l.SourceURL, l.PublishedAt = textPtr(key), textPtr(url), datePtr(published)
	l.FirstSeenAt, l.LastSeenAt = l.FirstSeenAt.UTC(), l.LastSeenAt.UTC()
	return l, nil
}

func (s *store) InsertListing(ctx context.Context, l domain.Listing) error {
	_, err := s.q.Exec(ctx, `
		INSERT INTO core.lot_listings (id, lot_id, data_source_id, source_key, source_url, published_at, first_seen_at, last_seen_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		uuidArg(l.ID), uuidArg(l.LotID), uuidArg(l.DataSourceID), textArg(l.SourceKey), textArg(l.SourceURL),
		dateArg(l.PublishedAt), l.FirstSeenAt, l.LastSeenAt, l.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert listing: %w", err)
	}
	return nil
}

func (s *store) UpdateListing(ctx context.Context, l domain.Listing) error {
	tag, err := s.q.Exec(ctx, `
		UPDATE core.lot_listings
		SET source_key = $2, source_url = $3, published_at = $4, first_seen_at = $5, last_seen_at = $6, updated_at = now()
		WHERE id = $1`,
		uuidArg(l.ID), textArg(l.SourceKey), textArg(l.SourceURL), dateArg(l.PublishedAt), l.FirstSeenAt, l.LastSeenAt)
	if err != nil {
		return fmt.Errorf("update listing: %w", err)
	}
	return requireAffected(tag, "listing", l.ID)
}

func (s *store) findListing(ctx context.Context, where string, args ...any) (*domain.Listing, error) {
	l, err := scanListing(s.q.QueryRow(ctx, `SELECT `+listingColumns+` FROM core.lot_listings li WHERE `+where, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find listing: %w", err)
	}
	return &l, nil
}

func (s *store) FindListing(ctx context.Context, lotID, dataSourceID domain.ID) (*domain.Listing, error) {
	return s.findListing(ctx, `li.lot_id = $1 AND li.data_source_id = $2`, uuidArg(lotID), uuidArg(dataSourceID))
}

func (s *store) FindListingBySourceKey(ctx context.Context, dataSourceID domain.ID, key string) (*domain.Listing, error) {
	return s.findListing(ctx, `li.data_source_id = $1 AND li.source_key = $2`, uuidArg(dataSourceID), key)
}

func (s *store) ListListings(ctx context.Context, lotID domain.ID) ([]app.ListingView, error) {
	rows, err := s.q.Query(ctx, `
		SELECT `+listingColumns+`, ds.code, ds.name
		FROM core.lot_listings li JOIN core.data_sources ds ON ds.id = li.data_source_id
		WHERE li.lot_id = $1 ORDER BY li.created_at, li.id`, uuidArg(lotID))
	if err != nil {
		return nil, fmt.Errorf("list listings: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (app.ListingView, error) {
		var v app.ListingView
		l, err := scanListing(row, &v.SourceCode, &v.SourceName)
		v.Listing = l
		return v, err
	})
}

// ---- batches -------------------------------------------------------------------

const batchColumns = `id, data_source_id, origin_kind, origin_ref, observed_at, observed_at_basis,
	scope_building_id, scope_lot_types, completeness, confirmed_at, content_hash, received_at`

// originRefUniqueConstraint is the unique key that makes a repeated origin
// reference impossible.
const originRefUniqueConstraint = "ingestion_batches_origin_kind_origin_ref_key"

func (s *store) InsertBatch(ctx context.Context, b domain.IngestionBatch) error {
	lotTypes := make([]string, 0, len(b.Scope.LotTypes))
	for _, t := range b.Scope.LotTypes {
		lotTypes = append(lotTypes, string(t))
	}
	if len(lotTypes) == 0 {
		lotTypes = nil
	}
	_, err := s.q.Exec(ctx, `
		INSERT INTO core.ingestion_batches (`+batchColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		uuidArg(b.ID), uuidArg(b.DataSourceID), string(b.OriginKind), nullUUIDArg(b.OriginRef),
		b.ObservedAt, string(b.ObservedAtBasis), nullUUIDArg(b.Scope.BuildingID), lotTypes,
		string(b.Completeness), b.ConfirmedAt, b.ContentHash, b.ReceivedAt)
	if err != nil {
		// Another transaction applied a batch with the same origin reference
		// concurrently (possible only for a different project, which the
		// project lock does not serialize).
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == originRefUniqueConstraint {
			return &domain.RuleError{Code: domain.CodeOriginConflict, Field: "origin_ref",
				Detail: "a batch with the same origin reference was applied concurrently"}
		}
		return fmt.Errorf("insert batch: %w", err)
	}
	return nil
}

func (s *store) FindBatchByOrigin(ctx context.Context, kind domain.OriginKind, ref domain.ID) (*domain.IngestionBatch, error) {
	var b domain.IngestionBatch
	var id, source, origin, scopeBuilding pgtype.UUID
	var k, basis, completeness string
	var lotTypes []string
	var confirmedAt pgtype.Timestamptz
	err := s.q.QueryRow(ctx, `SELECT `+batchColumns+`
		FROM core.ingestion_batches WHERE origin_kind = $1 AND origin_ref = $2`, string(kind), uuidArg(ref)).
		Scan(&id, &source, &k, &origin, &b.ObservedAt, &basis, &scopeBuilding, &lotTypes,
			&completeness, &confirmedAt, &b.ContentHash, &b.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find batch: %w", err)
	}
	b.ID, b.DataSourceID, b.OriginKind, b.OriginRef = idFrom(id), idFrom(source), domain.OriginKind(k), idPtrFrom(origin)
	b.ObservedAtBasis, b.Completeness = domain.DateBasis(basis), domain.Completeness(completeness)
	b.Scope.BuildingID = idPtrFrom(scopeBuilding)
	for _, t := range lotTypes {
		b.Scope.LotTypes = append(b.Scope.LotTypes, domain.LotType(t))
	}
	b.ConfirmedAt = timestampPtr(confirmedAt)
	return &b, nil
}

// ---- versions ------------------------------------------------------------------

const versionColumns = `v.id, v.listing_id, v.valid_from, v.valid_to, v.last_observed_at, v.presence, v.status,
	v.rooms, v.area_total::text, v.price::text, v.price_on_request, v.price_before_discount::text, v.finish, v.batch_id, v.created_at`

// versionScan receives nullable version columns (a LEFT JOIN may yield no version).
type versionScan struct {
	id, listing, batch             pgtype.UUID
	validFrom, validTo, lastObs    pgtype.Timestamptz
	createdAt                      pgtype.Timestamptz
	presence, status, finish       pgtype.Text
	area, price, priceBeforeDiscnt pgtype.Text
	priceOnRequest                 pgtype.Bool
	rooms                          pgtype.Int2
}

func (vs *versionScan) dest() []any {
	return []any{&vs.id, &vs.listing, &vs.validFrom, &vs.validTo, &vs.lastObs, &vs.presence, &vs.status,
		&vs.rooms, &vs.area, &vs.price, &vs.priceOnRequest, &vs.priceBeforeDiscnt, &vs.finish, &vs.batch, &vs.createdAt}
}

func (vs *versionScan) version() (*domain.Version, error) {
	if !vs.id.Valid {
		return nil, nil
	}
	v := domain.Version{
		ID:             idFrom(vs.id),
		ListingID:      idFrom(vs.listing),
		ValidFrom:      vs.validFrom.Time.UTC(),
		ValidTo:        timestampPtr(vs.validTo),
		LastObservedAt: vs.lastObs.Time.UTC(),
		BatchID:        idFrom(vs.batch),
		CreatedAt:      vs.createdAt.Time.UTC(),
		State: domain.LotState{
			Presence:       domain.Presence(vs.presence.String),
			Status:         domain.LotStatus(vs.status.String),
			Rooms:          intPtr(vs.rooms),
			PriceOnRequest: vs.priceOnRequest.Bool,
			Finish:         textPtr(vs.finish),
		},
	}
	var err error
	if v.State.AreaTotal, err = areaPtr(vs.area); err != nil {
		return nil, err
	}
	if v.State.Price, err = moneyPtr(vs.price); err != nil {
		return nil, err
	}
	if v.State.PriceBeforeDiscount, err = moneyPtr(vs.priceBeforeDiscnt); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *store) LockCurrentVersion(ctx context.Context, listingID domain.ID) (*domain.Version, error) {
	var vs versionScan
	err := s.q.QueryRow(ctx, `SELECT `+versionColumns+` FROM core.lot_listing_versions v
		WHERE v.listing_id = $1 AND v.valid_to IS NULL FOR UPDATE`, uuidArg(listingID)).Scan(vs.dest()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock current version: %w", err)
	}
	return vs.version()
}

func (s *store) InsertVersion(ctx context.Context, v domain.Version) error {
	st := v.State
	_, err := s.q.Exec(ctx, `
		INSERT INTO core.lot_listing_versions
		    (id, listing_id, valid_from, valid_to, last_observed_at, presence, status, rooms, area_total, price,
		     price_on_request, price_before_discount, finish, batch_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::numeric, $10::numeric, $11, $12::numeric, $13, $14, $15)`,
		uuidArg(v.ID), uuidArg(v.ListingID), v.ValidFrom, v.ValidTo, v.LastObservedAt, string(st.Presence), string(st.Status),
		intArg(st.Rooms), areaArg(st.AreaTotal), moneyArg(st.Price), st.PriceOnRequest, moneyArg(st.PriceBeforeDiscount), textArg(st.Finish),
		uuidArg(v.BatchID), v.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert version: %w", err)
	}
	return nil
}

func (s *store) CloseVersion(ctx context.Context, id domain.ID, validTo time.Time) error {
	tag, err := s.q.Exec(ctx, `UPDATE core.lot_listing_versions SET valid_to = $2 WHERE id = $1 AND valid_to IS NULL`,
		uuidArg(id), validTo)
	if err != nil {
		return fmt.Errorf("close version: %w", err)
	}
	return requireAffected(tag, "current version", id)
}

func (s *store) UpdateVersionLastObserved(ctx context.Context, id domain.ID, at time.Time) error {
	tag, err := s.q.Exec(ctx, `UPDATE core.lot_listing_versions SET last_observed_at = $2 WHERE id = $1`, uuidArg(id), at)
	if err != nil {
		return fmt.Errorf("update last observation: %w", err)
	}
	return requireAffected(tag, "version", id)
}

func (s *store) ListVersions(ctx context.Context, listingID domain.ID) ([]domain.Version, error) {
	rows, err := s.q.Query(ctx, `SELECT `+versionColumns+` FROM core.lot_listing_versions v
		WHERE v.listing_id = $1 ORDER BY v.valid_from`, uuidArg(listingID))
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Version, error) {
		var vs versionScan
		if err := row.Scan(vs.dest()...); err != nil {
			return domain.Version{}, err
		}
		v, err := vs.version()
		if err != nil {
			return domain.Version{}, err
		}
		return *v, nil
	})
}

// ---- relations and portfolio ------------------------------------------------------

func (s *store) InsertRelation(ctx context.Context, r domain.ProjectRelation) error {
	_, err := s.q.Exec(ctx, `
		INSERT INTO core.project_relations (id, subject_project_id, related_project_id, relation_type, rationale, valid_from, valid_to, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		uuidArg(r.ID), uuidArg(r.SubjectProjectID), uuidArg(r.RelatedProjectID), string(r.Type),
		optionalTextArg(r.Rationale), dateArg(&r.ValidFrom), dateArg(r.ValidTo), r.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert relation: %w", err)
	}
	return nil
}

func (s *store) ListRelationsBySubject(ctx context.Context, subjectID domain.ID) ([]domain.ProjectRelation, error) {
	return s.listRelations(ctx, `subject_project_id = $1`, uuidArg(subjectID))
}

func (s *store) ListRelationsByRelated(ctx context.Context, relatedID domain.ID) ([]domain.ProjectRelation, error) {
	return s.listRelations(ctx, `related_project_id = $1`, uuidArg(relatedID))
}

func (s *store) listRelations(ctx context.Context, where string, args ...any) ([]domain.ProjectRelation, error) {
	rows, err := s.q.Query(ctx, `
		SELECT id, subject_project_id, related_project_id, relation_type, COALESCE(rationale, ''), valid_from, valid_to, created_at
		FROM core.project_relations WHERE `+where+`
		ORDER BY valid_from DESC, relation_type, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list relations: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.ProjectRelation, error) {
		var r domain.ProjectRelation
		var id, subject, related pgtype.UUID
		var typ string
		var from, to pgtype.Date
		if err := row.Scan(&id, &subject, &related, &typ, &r.Rationale, &from, &to, &r.CreatedAt); err != nil {
			return r, err
		}
		r.ID, r.SubjectProjectID, r.RelatedProjectID, r.Type = idFrom(id), idFrom(subject), idFrom(related), domain.RelationType(typ)
		r.ValidFrom, r.ValidTo = domain.DateOf(from.Time), datePtr(to)
		return r, nil
	})
}

func (s *store) InsertPortfolioMembership(ctx context.Context, m domain.PortfolioMembership) error {
	_, err := s.q.Exec(ctx, `
		INSERT INTO core.portfolio_memberships (id, project_id, valid_from, valid_to, note, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		uuidArg(m.ID), uuidArg(m.ProjectID), dateArg(m.ValidFrom), dateArg(m.ValidTo), optionalTextArg(m.Note), m.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert portfolio membership: %w", err)
	}
	return nil
}

func (s *store) ListPortfolioMemberships(ctx context.Context, projectID domain.ID) ([]domain.PortfolioMembership, error) {
	rows, err := s.q.Query(ctx, `
		SELECT id, project_id, valid_from, valid_to, COALESCE(note, ''), created_at
		FROM core.portfolio_memberships WHERE project_id = $1
		ORDER BY valid_from NULLS FIRST, id`, uuidArg(projectID))
	if err != nil {
		return nil, fmt.Errorf("list portfolio memberships: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.PortfolioMembership, error) {
		var m domain.PortfolioMembership
		var id, project pgtype.UUID
		var from, to pgtype.Date
		if err := row.Scan(&id, &project, &from, &to, &m.Note, &m.CreatedAt); err != nil {
			return m, err
		}
		m.ID, m.ProjectID, m.ValidFrom, m.ValidTo = idFrom(id), idFrom(project), datePtr(from), datePtr(to)
		return m, nil
	})
}
