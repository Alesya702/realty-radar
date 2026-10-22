package core

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	httpadapter "realtyradar/src/modules/core/internal/adapters/http"
	"realtyradar/src/modules/core/internal/adapters/postgres"
	"realtyradar/src/modules/core/internal/app"
	"realtyradar/src/modules/core/internal/domain"
	"realtyradar/src/web"
)

// Module is the Core module: its use cases (the public Go contract for other
// modules) and its pages.
type Module struct {
	*app.Service
	pages *httpadapter.Handler
}

// New wires the Core module on a PostgreSQL pool. nav is the UI shell navigation.
func New(pool *pgxpool.Pool, logger *slog.Logger, nav []web.Link) (*Module, error) {
	svc := app.NewService(postgres.New(pool))
	pages, err := httpadapter.New(svc, logger, nav)
	if err != nil {
		return nil, fmt.Errorf("core pages: %w", err)
	}
	return &Module{Service: svc, pages: pages}, nil
}

// Nav returns the navigation links of the Core pages.
func Nav() []web.Link { return httpadapter.Nav() }

// RegisterRoutes registers the Core pages.
func (m *Module) RegisterRoutes(mux *http.ServeMux) { m.pages.Register(mux) }

// Public contract types. They are aliases of the module's internal types so
// that other modules can use Core values without importing internal packages.
type (
	// ID is a canonical Realty Radar identifier (UUIDv7).
	ID = domain.ID
	// Money is a ruble amount in kopecks.
	Money = domain.Money
	// Area is an area in hundredths of m².
	Area = domain.Area

	LotType        = domain.LotType
	LotStatus      = domain.LotStatus
	Presence       = domain.Presence
	RelationType   = domain.RelationType
	DataSourceKind = domain.DataSourceKind
	OriginKind     = domain.OriginKind
	ChangeKind     = domain.ChangeKind
	// DateBasis is what the moment of a batch is based on.
	DateBasis = domain.DateBasis
	// Completeness tells whether a batch contains all lots of its scope.
	Completeness = domain.Completeness
	// BatchScope is the part of a project a batch describes.
	BatchScope = domain.BatchScope

	// Field is an observed value: known, unknown (keeps the previous value) or absent.
	Field[T comparable] = domain.Field[T]
	ObservedState       = domain.ObservedState
	RuleError           = domain.RuleError

	Developer           = domain.Developer
	Project             = domain.Project
	Building            = domain.Building
	Section             = domain.Section
	Lot                 = domain.Lot
	Placement           = domain.Placement
	Listing             = domain.Listing
	Version             = domain.Version
	LotState            = domain.LotState
	DataSource          = domain.DataSource
	ProjectRelation     = domain.ProjectRelation
	PortfolioMembership = domain.PortfolioMembership

	CreateProjectInput   = app.CreateProjectInput
	UpdateProjectInput   = app.UpdateProjectInput
	AddToPortfolioInput  = app.AddToPortfolioInput
	RelationInput        = app.RelationInput
	ProjectSummary       = app.ProjectSummary
	BuildingView         = app.BuildingView
	RelationView         = app.RelationView
	IncomingRelationView = app.IncomingRelationView
	LotRow               = app.LotRow
	LotHistory           = app.LotHistory
	ListingView          = app.ListingView

	// ObservationBatch is the ingestion contract for confirmed observations
	// (manual entry now, PDF import next).
	ObservationBatch = app.ObservationBatch
	BuildingDecision = app.BuildingDecision
	SectionDecision  = app.SectionDecision
	LotTarget        = app.LotTarget
	NewLotPlacement  = app.NewLotPlacement
	LotObservation   = app.LotObservation
	BatchResult      = app.BatchResult
	ItemResult       = app.ItemResult
	ItemError        = app.ItemError
	// BatchError reports every item-level rule violation of one batch.
	BatchError = app.BatchError
)

// Enumerations of the public contract.
const (
	LotTypeApartment  = domain.LotTypeApartment
	LotTypeCommercial = domain.LotTypeCommercial
	LotTypeStorage    = domain.LotTypeStorage
	LotTypeParking    = domain.LotTypeParking

	LotStatusAvailable = domain.LotStatusAvailable
	LotStatusReserved  = domain.LotStatusReserved
	LotStatusSold      = domain.LotStatusSold
	LotStatusUnknown   = domain.LotStatusUnknown

	PresenceListed   = domain.PresenceListed
	PresenceDelisted = domain.PresenceDelisted

	RelationDirectCompetitor   = domain.RelationDirectCompetitor
	RelationIndirectCompetitor = domain.RelationIndirectCompetitor
	RelationReference          = domain.RelationReference

	DataSourceManual          = domain.DataSourceManual
	DataSourceDocumentChannel = domain.DataSourceDocumentChannel
	ManualDataSourceCode      = domain.ManualDataSourceCode

	OriginManualEntry    = domain.OriginManualEntry
	OriginDocumentImport = domain.OriginDocumentImport

	BasisDocumentDate = domain.BasisDocumentDate
	BasisReceivedDate = domain.BasisReceivedDate
	BasisUploadTime   = domain.BasisUploadTime
	BasisEntryTime    = domain.BasisEntryTime

	CompletenessPartial  = domain.CompletenessPartial
	CompletenessComplete = domain.CompletenessComplete

	ChangeCreated   = domain.ChangeCreated
	ChangeChanged   = domain.ChangeChanged
	ChangeConfirmed = domain.ChangeConfirmed
)

// ErrNotFound is returned (wrapped) when a requested object does not exist.
var ErrNotFound = domain.ErrNotFound

// Value returns an observed field with a known value.
func Value[T comparable](v T) Field[T] { return domain.Value(v) }

// Absent returns an observed field the source explicitly shows as absent.
func Absent[T comparable]() Field[T] { return domain.Absent[T]() }

// Unknown returns an observed field the source does not report.
func Unknown[T comparable]() Field[T] { return domain.Unknown[T]() }

// ParseID parses a canonical identifier.
func ParseID(s string) (ID, error) { return domain.ParseID(s) }

// ParseMoney parses a ruble amount such as "15 900 000".
func ParseMoney(s string) (Money, error) { return domain.ParseMoney(s) }

// ParseArea parses an area such as "67,4".
func ParseArea(s string) (Area, error) { return domain.ParseArea(s) }

// AsRuleError returns the domain rule violation wrapped in err, if any.
func AsRuleError(err error) (*RuleError, bool) { return domain.AsRuleError(err) }
