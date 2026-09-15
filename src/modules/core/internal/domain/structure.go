package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxNameLength   = 300
	maxNumberLength = 50
)

// Developer is a market participant on whose behalf projects are developed.
// Its exact level (brand, group, legal entity) is open: Q-CORE-21.
type Developer struct {
	ID        ID
	Name      string
	CreatedAt time.Time
}

// NewDeveloper validates input and creates a developer.
func NewDeveloper(id ID, name string, now time.Time) (Developer, error) {
	name, err := requiredText("name", name, maxNameLength)
	if err != nil {
		return Developer{}, err
	}
	return Developer{ID: id, Name: name, CreatedAt: now}, nil
}

// Project is a real estate project (residential complex) regardless of its
// role for the company: own and external projects live in one model
// (core-domain-model.md, 4.1). The role is expressed by PortfolioMembership and
// ProjectRelation, never by a flag on the project.
type Project struct {
	ID          ID
	Name        string
	DeveloperID *ID
	City        string // optional; stored as data (A-10)
	CreatedAt   time.Time
}

// NewProject validates input and creates a project.
func NewProject(id ID, name string, developerID *ID, city string, now time.Time) (Project, error) {
	name, err := requiredText("name", name, maxNameLength)
	if err != nil {
		return Project{}, err
	}
	city, err = optionalText("city", city, maxNameLength)
	if err != nil {
		return Project{}, err
	}
	return Project{ID: id, Name: name, DeveloperID: developerID, City: city, CreatedAt: now}, nil
}

// Building is a building of a project. A project may have no buildings.
type Building struct {
	ID        ID
	ProjectID ID
	Name      string
	CreatedAt time.Time
}

// NewBuilding validates input and creates a building within a project.
func NewBuilding(id, projectID ID, name string, now time.Time) (Building, error) {
	name, err := requiredText("name", name, maxNameLength)
	if err != nil {
		return Building{}, err
	}
	return Building{ID: id, ProjectID: projectID, Name: name, CreatedAt: now}, nil
}

// Section is an optional part of a building. It always belongs to a building.
type Section struct {
	ID         ID
	BuildingID ID
	Name       string
	CreatedAt  time.Time
}

// NewSection validates input and creates a section within a building.
func NewSection(id, buildingID ID, name string, now time.Time) (Section, error) {
	name, err := requiredText("name", name, maxNameLength)
	if err != nil {
		return Section{}, err
	}
	return Section{ID: id, BuildingID: buildingID, Name: name, CreatedAt: now}, nil
}

// DataSourceKind classifies the origin of data. Only kinds needed by the MVP
// exist; website and aggregator are future scope.
type DataSourceKind string

// Data source kinds.
const (
	DataSourceManual          DataSourceKind = "manual"
	DataSourceDocumentChannel DataSourceKind = "document_channel"
)

// Valid reports whether the kind is known.
func (k DataSourceKind) Valid() bool {
	return k == DataSourceManual || k == DataSourceDocumentChannel
}

// ManualDataSourceCode is the code of the data source seeded by the Core
// migration for manual entry.
const ManualDataSourceCode = "manual"

// DataSource is the identity of a data origin (ADR-0006). It contains no
// logic of a particular source.
type DataSource struct {
	ID        ID
	Code      string
	Name      string
	Kind      DataSourceKind
	IsActive  bool
	CreatedAt time.Time
}

var dataSourceCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,62}$`)

// NewDataSource validates input and creates an active data source.
func NewDataSource(id ID, code, name string, kind DataSourceKind, now time.Time) (DataSource, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return DataSource{}, ruleErr(CodeRequired, "code", "code is required")
	}
	if !dataSourceCodePattern.MatchString(code) {
		return DataSource{}, ruleErr(CodeInvalidValue, "code", "code must match "+dataSourceCodePattern.String())
	}
	name, err := requiredText("name", name, maxNameLength)
	if err != nil {
		return DataSource{}, err
	}
	if !kind.Valid() {
		return DataSource{}, ruleErr(CodeInvalidValue, "kind", "unknown data source kind "+string(kind))
	}
	return DataSource{ID: id, Code: code, Name: name, Kind: kind, IsActive: true, CreatedAt: now}, nil
}

func requiredText(field, value string, maxLen int) (string, error) {
	if !utf8.ValidString(value) {
		return "", ruleErr(CodeInvalidValue, field, field+" is not valid UTF-8")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ruleErr(CodeRequired, field, field+" is required")
	}
	if utf8.RuneCountInString(value) > maxLen {
		return "", ruleErr(CodeTooLong, field, field+" is too long")
	}
	return value, nil
}

func optionalText(field, value string, maxLen int) (string, error) {
	if !utf8.ValidString(value) {
		return "", ruleErr(CodeInvalidValue, field, field+" is not valid UTF-8")
	}
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > maxLen {
		return "", ruleErr(CodeTooLong, field, field+" is too long")
	}
	return value, nil
}
