// Package postgres implements the Core persistence port on PostgreSQL (schema
// core) with pgx. All SQL of the Core module lives in this package.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"realtyradar/src/modules/core/internal/app"
	"realtyradar/src/modules/core/internal/domain"
)

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository implements app.Repository.
type Repository struct {
	store
	pool *pgxpool.Pool
}

var _ app.Repository = (*Repository)(nil)

// New creates a repository on the connection pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{store: store{q: pool}, pool: pool}
}

// InTx runs fn in a transaction.
func (r *Repository) InTx(ctx context.Context, fn func(app.Store) error) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		return fn(&store{q: tx})
	})
}

type store struct {
	q querier
}

// ---- helpers ---------------------------------------------------------------

func uuidArg(id domain.ID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func nullUUIDArg(id *domain.ID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func idFrom(u pgtype.UUID) domain.ID { return domain.ID(u.Bytes) }

func idPtrFrom(u pgtype.UUID) *domain.ID {
	if !u.Valid {
		return nil
	}
	id := domain.ID(u.Bytes)
	return &id
}

func textArg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func optionalTextArg(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func dateArg(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func datePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := domain.DateOf(d.Time)
	return &t
}

func timestampPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func intArg(v *int) pgtype.Int2 {
	if v == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: int16(*v), Valid: true}
}

func intPtr(v pgtype.Int2) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int16)
	return &i
}

// Numeric values travel as decimal text to avoid floating point (A-08). A Go
// string is sent in text format, which PostgreSQL parses as numeric.
func moneyArg(m *domain.Money) any {
	if m == nil {
		return nil
	}
	return m.String()
}

func areaArg(a *domain.Area) any {
	if a == nil {
		return nil
	}
	return a.String()
}

func moneyPtr(t pgtype.Text) (*domain.Money, error) {
	if !t.Valid {
		return nil, nil
	}
	m, err := domain.ParseMoney(t.String)
	if err != nil {
		return nil, fmt.Errorf("decode money %q: %w", t.String, err)
	}
	return &m, nil
}

func areaPtr(t pgtype.Text) (*domain.Area, error) {
	if !t.Valid {
		return nil, nil
	}
	a, err := domain.ParseArea(t.String)
	if err != nil {
		return nil, fmt.Errorf("decode area %q: %w", t.String, err)
	}
	return &a, nil
}

func notFound(err error, what string, id domain.ID) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s %s: %w", what, id, domain.ErrNotFound)
	}
	return fmt.Errorf("load %s %s: %w", what, id, err)
}

func requireAffected(tag pgconn.CommandTag, what string, id domain.ID) error {
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s %s: %w", what, id, domain.ErrNotFound)
	}
	return nil
}

// ---- developers --------------------------------------------------------------

func (s *store) InsertDeveloper(ctx context.Context, d domain.Developer) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO core.developers (id, name, created_at, updated_at) VALUES ($1, $2, $3, $3)`,
		uuidArg(d.ID), d.Name, d.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert developer: %w", err)
	}
	return nil
}

func (s *store) GetDeveloper(ctx context.Context, id domain.ID) (domain.Developer, error) {
	var d domain.Developer
	var rid pgtype.UUID
	err := s.q.QueryRow(ctx, `SELECT id, name, created_at FROM core.developers WHERE id = $1`, uuidArg(id)).
		Scan(&rid, &d.Name, &d.CreatedAt)
	if err != nil {
		return domain.Developer{}, notFound(err, "developer", id)
	}
	d.ID = idFrom(rid)
	return d, nil
}

func (s *store) ListDevelopers(ctx context.Context) ([]domain.Developer, error) {
	rows, err := s.q.Query(ctx, `SELECT id, name, created_at FROM core.developers ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list developers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Developer, error) {
		var d domain.Developer
		var rid pgtype.UUID
		err := row.Scan(&rid, &d.Name, &d.CreatedAt)
		d.ID = idFrom(rid)
		return d, err
	})
}

// ---- projects ------------------------------------------------------------------

func (s *store) InsertProject(ctx context.Context, p domain.Project) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO core.projects (id, developer_id, name, city, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $5)`,
		uuidArg(p.ID), nullUUIDArg(p.DeveloperID), p.Name, optionalTextArg(p.City), p.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *store) UpdateProjectDeveloper(ctx context.Context, projectID domain.ID, developerID *domain.ID) error {
	tag, err := s.q.Exec(ctx, `UPDATE core.projects SET developer_id = $2, updated_at = now() WHERE id = $1`,
		uuidArg(projectID), nullUUIDArg(developerID))
	if err != nil {
		return fmt.Errorf("update developer of project %s: %w", projectID, err)
	}
	return requireAffected(tag, "project", projectID)
}

func (s *store) UpdateProject(ctx context.Context, p domain.Project) error {
	tag, err := s.q.Exec(ctx, `UPDATE core.projects SET name = $2, city = $3, developer_id = $4, updated_at = now() WHERE id = $1`,
		uuidArg(p.ID), p.Name, optionalTextArg(p.City), nullUUIDArg(p.DeveloperID))
	if err != nil {
		return fmt.Errorf("update project %s: %w", p.ID, err)
	}
	return requireAffected(tag, "project", p.ID)
}

const projectColumns = `p.id, p.developer_id, p.name, COALESCE(p.city, ''), p.created_at`

func scanProject(row pgx.Row, extra ...any) (domain.Project, error) {
	var p domain.Project
	var id, dev pgtype.UUID
	dest := append([]any{&id, &dev, &p.Name, &p.City, &p.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return domain.Project{}, err
	}
	p.ID, p.DeveloperID = idFrom(id), idPtrFrom(dev)
	return p, nil
}

func (s *store) GetProject(ctx context.Context, id domain.ID) (domain.Project, error) {
	p, err := scanProject(s.q.QueryRow(ctx, `SELECT `+projectColumns+` FROM core.projects p WHERE p.id = $1`, uuidArg(id)))
	if err != nil {
		return domain.Project{}, notFound(err, "project", id)
	}
	return p, nil
}

// LockProject serializes the writers of one project. FOR NO KEY UPDATE is the
// weakest lock that conflicts with itself, so two batches of the same project
// still wait for each other; unlike FOR UPDATE it does not conflict with the
// FOR KEY SHARE lock PostgreSQL takes on the project row when another
// transaction inserts a row referencing the project (relations, portfolio,
// buildings, lots), which otherwise deadlocks (review M5).
func (s *store) LockProject(ctx context.Context, id domain.ID) error {
	var rid pgtype.UUID
	err := s.q.QueryRow(ctx, `SELECT id FROM core.projects WHERE id = $1 FOR NO KEY UPDATE`, uuidArg(id)).Scan(&rid)
	if err != nil {
		return notFound(err, "project", id)
	}
	return nil
}

func (s *store) ListProjects(ctx context.Context, portfolioOn time.Time, onlyPortfolio bool) ([]app.ProjectSummary, error) {
	rows, err := s.q.Query(ctx, `
		SELECT `+projectColumns+`,
		       COALESCE(d.name, ''),
		       EXISTS (SELECT 1 FROM core.portfolio_memberships m
		               WHERE m.project_id = p.id
		                 AND (m.valid_from IS NULL OR m.valid_from <= $1)
		                 AND (m.valid_to IS NULL OR m.valid_to >= $1)) AS in_portfolio,
		       (SELECT count(*) FROM core.lots l WHERE l.project_id = p.id) AS lot_count
		FROM core.projects p
		LEFT JOIN core.developers d ON d.id = p.developer_id
		ORDER BY p.name, p.id`, pgtype.Date{Time: portfolioOn, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (app.ProjectSummary, error) {
		var sum app.ProjectSummary
		var lotCount int64
		p, err := scanProject(row, &sum.DeveloperName, &sum.InPortfolio, &lotCount)
		sum.Project, sum.LotCount = p, int(lotCount)
		return sum, err
	})
	if err != nil || !onlyPortfolio {
		return all, err
	}
	filtered := all[:0]
	for _, p := range all {
		if p.InPortfolio {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

// ---- buildings and sections -----------------------------------------------------

func (s *store) InsertBuilding(ctx context.Context, b domain.Building) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO core.buildings (id, project_id, name, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`,
		uuidArg(b.ID), uuidArg(b.ProjectID), b.Name, b.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert building: %w", err)
	}
	return nil
}

func scanBuilding(row pgx.Row) (domain.Building, error) {
	var b domain.Building
	var id, project pgtype.UUID
	if err := row.Scan(&id, &project, &b.Name, &b.CreatedAt); err != nil {
		return domain.Building{}, err
	}
	b.ID, b.ProjectID = idFrom(id), idFrom(project)
	return b, nil
}

func (s *store) GetBuilding(ctx context.Context, id domain.ID) (domain.Building, error) {
	b, err := scanBuilding(s.q.QueryRow(ctx, `SELECT id, project_id, name, created_at FROM core.buildings WHERE id = $1`, uuidArg(id)))
	if err != nil {
		return domain.Building{}, notFound(err, "building", id)
	}
	return b, nil
}

func (s *store) FindBuildingByName(ctx context.Context, projectID domain.ID, name string) (*domain.Building, error) {
	b, err := scanBuilding(s.q.QueryRow(ctx,
		`SELECT id, project_id, name, created_at FROM core.buildings WHERE project_id = $1 AND name = $2`, uuidArg(projectID), name))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find building %q: %w", name, err)
	}
	return &b, nil
}

func (s *store) ListBuildings(ctx context.Context, projectID domain.ID) ([]domain.Building, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, project_id, name, created_at FROM core.buildings WHERE project_id = $1 ORDER BY name, id`, uuidArg(projectID))
	if err != nil {
		return nil, fmt.Errorf("list buildings: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Building, error) { return scanBuilding(row) })
}

func (s *store) InsertSection(ctx context.Context, sec domain.Section) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO core.sections (id, building_id, name, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`,
		uuidArg(sec.ID), uuidArg(sec.BuildingID), sec.Name, sec.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert section: %w", err)
	}
	return nil
}

func scanSection(row pgx.Row) (domain.Section, error) {
	var sec domain.Section
	var id, building pgtype.UUID
	if err := row.Scan(&id, &building, &sec.Name, &sec.CreatedAt); err != nil {
		return domain.Section{}, err
	}
	sec.ID, sec.BuildingID = idFrom(id), idFrom(building)
	return sec, nil
}

func (s *store) GetSection(ctx context.Context, id domain.ID) (domain.Section, error) {
	sec, err := scanSection(s.q.QueryRow(ctx, `SELECT id, building_id, name, created_at FROM core.sections WHERE id = $1`, uuidArg(id)))
	if err != nil {
		return domain.Section{}, notFound(err, "section", id)
	}
	return sec, nil
}

func (s *store) FindSectionByName(ctx context.Context, buildingID domain.ID, name string) (*domain.Section, error) {
	sec, err := scanSection(s.q.QueryRow(ctx,
		`SELECT id, building_id, name, created_at FROM core.sections WHERE building_id = $1 AND name = $2`, uuidArg(buildingID), name))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find section %q: %w", name, err)
	}
	return &sec, nil
}

func (s *store) ListSectionsByProject(ctx context.Context, projectID domain.ID) ([]domain.Section, error) {
	rows, err := s.q.Query(ctx, `
		SELECT s.id, s.building_id, s.name, s.created_at
		FROM core.sections s JOIN core.buildings b ON b.id = s.building_id
		WHERE b.project_id = $1 ORDER BY s.name, s.id`, uuidArg(projectID))
	if err != nil {
		return nil, fmt.Errorf("list sections: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Section, error) { return scanSection(row) })
}

// ---- data sources ----------------------------------------------------------------

const dataSourceColumns = `id, code, name, kind, is_active, created_at`

func scanDataSource(row pgx.Row) (domain.DataSource, error) {
	var ds domain.DataSource
	var id pgtype.UUID
	var kind string
	if err := row.Scan(&id, &ds.Code, &ds.Name, &kind, &ds.IsActive, &ds.CreatedAt); err != nil {
		return domain.DataSource{}, err
	}
	ds.ID, ds.Kind = idFrom(id), domain.DataSourceKind(kind)
	return ds, nil
}

func (s *store) InsertDataSource(ctx context.Context, ds domain.DataSource) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO core.data_sources (id, code, name, kind, is_active, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		uuidArg(ds.ID), ds.Code, ds.Name, string(ds.Kind), ds.IsActive, ds.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert data source: %w", err)
	}
	return nil
}

func (s *store) GetDataSource(ctx context.Context, id domain.ID) (domain.DataSource, error) {
	ds, err := scanDataSource(s.q.QueryRow(ctx, `SELECT `+dataSourceColumns+` FROM core.data_sources WHERE id = $1`, uuidArg(id)))
	if err != nil {
		return domain.DataSource{}, notFound(err, "data source", id)
	}
	return ds, nil
}

func (s *store) FindDataSourceByCode(ctx context.Context, code string) (*domain.DataSource, error) {
	ds, err := scanDataSource(s.q.QueryRow(ctx, `SELECT `+dataSourceColumns+` FROM core.data_sources WHERE code = $1`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find data source %q: %w", code, err)
	}
	return &ds, nil
}

func (s *store) ListDataSources(ctx context.Context) ([]domain.DataSource, error) {
	rows, err := s.q.Query(ctx, `SELECT `+dataSourceColumns+` FROM core.data_sources ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("list data sources: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.DataSource, error) { return scanDataSource(row) })
}
