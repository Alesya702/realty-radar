// Package app contains the Core use cases and the ports (repository
// interfaces) they depend on. Adapters implement the ports; the module root
// package exposes the use cases to other modules.
package app

import (
	"context"
	"time"

	"realtyradar/src/modules/core/internal/domain"
)

// Store is the persistence port of Core. Methods return domain.ErrNotFound
// (possibly wrapped) when a single requested object does not exist; Find*
// methods return nil instead.
type Store interface {
	InsertDeveloper(ctx context.Context, d domain.Developer) error
	GetDeveloper(ctx context.Context, id domain.ID) (domain.Developer, error)
	ListDevelopers(ctx context.Context) ([]domain.Developer, error)

	InsertProject(ctx context.Context, p domain.Project) error
	UpdateProjectDeveloper(ctx context.Context, projectID domain.ID, developerID *domain.ID) error
	GetProject(ctx context.Context, id domain.ID) (domain.Project, error)
	// LockProject locks the project row for the rest of the transaction.
	LockProject(ctx context.Context, id domain.ID) error
	ListProjects(ctx context.Context, portfolioOn time.Time, onlyPortfolio bool) ([]ProjectSummary, error)

	InsertBuilding(ctx context.Context, b domain.Building) error
	GetBuilding(ctx context.Context, id domain.ID) (domain.Building, error)
	FindBuildingByName(ctx context.Context, projectID domain.ID, name string) (*domain.Building, error)
	ListBuildings(ctx context.Context, projectID domain.ID) ([]domain.Building, error)

	InsertSection(ctx context.Context, s domain.Section) error
	GetSection(ctx context.Context, id domain.ID) (domain.Section, error)
	FindSectionByName(ctx context.Context, buildingID domain.ID, name string) (*domain.Section, error)
	ListSectionsByProject(ctx context.Context, projectID domain.ID) ([]domain.Section, error)

	InsertDataSource(ctx context.Context, s domain.DataSource) error
	GetDataSource(ctx context.Context, id domain.ID) (domain.DataSource, error)
	FindDataSourceByCode(ctx context.Context, code string) (*domain.DataSource, error)
	ListDataSources(ctx context.Context) ([]domain.DataSource, error)

	InsertLot(ctx context.Context, l domain.Lot) error
	GetLot(ctx context.Context, id domain.ID) (domain.Lot, error)
	// ListLotsByNumber returns the lots of a project with the given lot type
	// and number, whatever their building and section: the candidates of key
	// K2 matching (domain.MatchPlacement).
	ListLotsByNumber(ctx context.Context, projectID domain.ID, lotType domain.LotType, number string) ([]domain.Lot, error)
	SetPrimaryListing(ctx context.Context, lotID, listingID domain.ID) error
	ListLotRows(ctx context.Context, projectID domain.ID) ([]LotRow, error)

	InsertListing(ctx context.Context, l domain.Listing) error
	UpdateListing(ctx context.Context, l domain.Listing) error
	FindListing(ctx context.Context, lotID, dataSourceID domain.ID) (*domain.Listing, error)
	FindListingBySourceKey(ctx context.Context, dataSourceID domain.ID, key string) (*domain.Listing, error)
	ListListings(ctx context.Context, lotID domain.ID) ([]ListingView, error)

	InsertBatch(ctx context.Context, b domain.IngestionBatch) error
	FindBatchByOrigin(ctx context.Context, kind domain.OriginKind, ref domain.ID) (*domain.IngestionBatch, error)

	// LockCurrentVersion returns the current version of a listing, locked for
	// update, or nil when the listing has no versions.
	LockCurrentVersion(ctx context.Context, listingID domain.ID) (*domain.Version, error)
	InsertVersion(ctx context.Context, v domain.Version) error
	CloseVersion(ctx context.Context, id domain.ID, validTo time.Time) error
	UpdateVersionLastObserved(ctx context.Context, id domain.ID, at time.Time) error
	ListVersions(ctx context.Context, listingID domain.ID) ([]domain.Version, error)

	InsertRelation(ctx context.Context, r domain.ProjectRelation) error
	ListRelationsBySubject(ctx context.Context, subjectID domain.ID) ([]domain.ProjectRelation, error)

	InsertPortfolioMembership(ctx context.Context, m domain.PortfolioMembership) error
	ListPortfolioMemberships(ctx context.Context, projectID domain.ID) ([]domain.PortfolioMembership, error)
}

// Repository is a Store that can run a function in a transaction.
type Repository interface {
	Store
	// InTx runs fn with a Store bound to one database transaction. The
	// transaction commits when fn returns nil and rolls back otherwise.
	InTx(ctx context.Context, fn func(Store) error) error
}

// ProjectSummary is a project row for lists.
type ProjectSummary struct {
	Project       domain.Project
	DeveloperName string
	InPortfolio   bool
	LotCount      int
}

// LotRow is a lot with its placement names and canonical current state.
type LotRow struct {
	Lot          domain.Lot
	BuildingName string
	SectionName  string
	Current      *domain.Version
}

// ListingView is a listing with its data source.
type ListingView struct {
	Listing    domain.Listing
	SourceCode string
	SourceName string
}
