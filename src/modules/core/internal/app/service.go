package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"realtyradar/src/modules/core/internal/domain"
)

// Service implements the Core use cases.
type Service struct {
	repo  Repository
	now   func() time.Time
	newID func() domain.ID
}

// Option customizes a Service (used by tests).
type Option func(*Service)

// WithClock sets the time source.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// NewService creates the Core use case service.
func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo, now: time.Now, newID: domain.NewID}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Service) clock() time.Time { return domain.NormalizeTime(s.now()) }

// CreateDeveloper registers a developer.
func (s *Service) CreateDeveloper(ctx context.Context, name string) (domain.Developer, error) {
	d, err := domain.NewDeveloper(s.newID(), name, s.clock())
	if err != nil {
		return domain.Developer{}, err
	}
	if err := s.repo.InsertDeveloper(ctx, d); err != nil {
		return domain.Developer{}, fmt.Errorf("create developer: %w", err)
	}
	return d, nil
}

// GetDeveloper returns a developer.
func (s *Service) GetDeveloper(ctx context.Context, id domain.ID) (domain.Developer, error) {
	return s.repo.GetDeveloper(ctx, id)
}

// ListDevelopers returns all developers ordered by name.
func (s *Service) ListDevelopers(ctx context.Context) ([]domain.Developer, error) {
	return s.repo.ListDevelopers(ctx)
}

// CreateProjectInput is the input of CreateProject.
type CreateProjectInput struct {
	Name        string
	DeveloperID *domain.ID
	City        string
}

// CreateProject registers a project, optionally associated with a developer.
func (s *Service) CreateProject(ctx context.Context, in CreateProjectInput) (domain.Project, error) {
	p, err := domain.NewProject(s.newID(), in.Name, in.DeveloperID, in.City, s.clock())
	if err != nil {
		return domain.Project{}, err
	}
	if in.DeveloperID != nil {
		if _, err := s.repo.GetDeveloper(ctx, *in.DeveloperID); err != nil {
			return domain.Project{}, fmt.Errorf("create project: developer %s: %w", in.DeveloperID, err)
		}
	}
	if err := s.repo.InsertProject(ctx, p); err != nil {
		return domain.Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// SetProjectDeveloper associates a project with a developer (nil clears it).
func (s *Service) SetProjectDeveloper(ctx context.Context, projectID domain.ID, developerID *domain.ID) error {
	if _, err := s.repo.GetProject(ctx, projectID); err != nil {
		return fmt.Errorf("set developer of project %s: %w", projectID, err)
	}
	if developerID != nil {
		if _, err := s.repo.GetDeveloper(ctx, *developerID); err != nil {
			return fmt.Errorf("set developer of project %s: developer %s: %w", projectID, developerID, err)
		}
	}
	return s.repo.UpdateProjectDeveloper(ctx, projectID, developerID)
}

// UpdateProjectInput is the input of UpdateProject.
type UpdateProjectInput struct {
	ProjectID   domain.ID
	Name        string
	DeveloperID *domain.ID
	City        string
}

// UpdateProject changes the name, city and developer of a project. The rules
// are those of CreateProject.
func (s *Service) UpdateProject(ctx context.Context, in UpdateProjectInput) (domain.Project, error) {
	var p domain.Project
	err := s.repo.InTx(ctx, func(st Store) error {
		if err := st.LockProject(ctx, in.ProjectID); err != nil {
			return fmt.Errorf("update project %s: %w", in.ProjectID, err)
		}
		current, err := st.GetProject(ctx, in.ProjectID)
		if err != nil {
			return fmt.Errorf("update project %s: %w", in.ProjectID, err)
		}
		p, err = domain.NewProject(current.ID, in.Name, in.DeveloperID, in.City, current.CreatedAt)
		if err != nil {
			return err
		}
		if in.DeveloperID != nil {
			if _, err := st.GetDeveloper(ctx, *in.DeveloperID); err != nil {
				return fmt.Errorf("update project %s: developer %s: %w", in.ProjectID, in.DeveloperID, err)
			}
		}
		return st.UpdateProject(ctx, p)
	})
	return p, err
}

// GetProject returns a project.
func (s *Service) GetProject(ctx context.Context, id domain.ID) (domain.Project, error) {
	return s.repo.GetProject(ctx, id)
}

// ListProjects returns all projects with developer name, portfolio flag (on
// the business date of the given moment) and lot count.
func (s *Service) ListProjects(ctx context.Context, on time.Time) ([]ProjectSummary, error) {
	return s.repo.ListProjects(ctx, domain.BusinessDateOf(on), false)
}

// ListPortfolioProjects returns projects that belong to the company portfolio
// on the business date of the given moment.
func (s *Service) ListPortfolioProjects(ctx context.Context, on time.Time) ([]ProjectSummary, error) {
	return s.repo.ListProjects(ctx, domain.BusinessDateOf(on), true)
}

// Today returns the current business date (A-07). Callers that need "now" as a
// business date use it instead of converting time zones themselves.
func (s *Service) Today() time.Time { return domain.BusinessDateOf(s.now()) }

// AddBuilding creates a building within a project.
func (s *Service) AddBuilding(ctx context.Context, projectID domain.ID, name string) (domain.Building, error) {
	var b domain.Building
	err := s.repo.InTx(ctx, func(st Store) error {
		var err error
		b, err = addBuilding(ctx, st, s.newID(), projectID, name, s.clock())
		return err
	})
	return b, err
}

func addBuilding(ctx context.Context, st Store, id, projectID domain.ID, name string, now time.Time) (domain.Building, error) {
	if err := st.LockProject(ctx, projectID); err != nil {
		return domain.Building{}, fmt.Errorf("add building to project %s: %w", projectID, err)
	}
	b, err := domain.NewBuilding(id, projectID, name, now)
	if err != nil {
		return domain.Building{}, err
	}
	existing, err := st.FindBuildingByName(ctx, projectID, b.Name)
	if err != nil {
		return domain.Building{}, fmt.Errorf("add building: %w", err)
	}
	if existing != nil {
		return domain.Building{}, &domain.RuleError{Code: domain.CodeNameTaken, Field: "name", Detail: "building " + b.Name + " already exists in the project"}
	}
	if err := st.InsertBuilding(ctx, b); err != nil {
		return domain.Building{}, fmt.Errorf("add building: %w", err)
	}
	return b, nil
}

// AddSection creates a section within a building.
func (s *Service) AddSection(ctx context.Context, buildingID domain.ID, name string) (domain.Section, error) {
	var sec domain.Section
	err := s.repo.InTx(ctx, func(st Store) error {
		b, err := st.GetBuilding(ctx, buildingID)
		if err != nil {
			return fmt.Errorf("add section to building %s: %w", buildingID, err)
		}
		sec, err = addSection(ctx, st, s.newID(), b, name, s.clock())
		return err
	})
	return sec, err
}

func addSection(ctx context.Context, st Store, id domain.ID, b domain.Building, name string, now time.Time) (domain.Section, error) {
	if err := st.LockProject(ctx, b.ProjectID); err != nil {
		return domain.Section{}, fmt.Errorf("add section: %w", err)
	}
	sec, err := domain.NewSection(id, b.ID, name, now)
	if err != nil {
		return domain.Section{}, err
	}
	existing, err := st.FindSectionByName(ctx, b.ID, sec.Name)
	if err != nil {
		return domain.Section{}, fmt.Errorf("add section: %w", err)
	}
	if existing != nil {
		return domain.Section{}, &domain.RuleError{Code: domain.CodeNameTaken, Field: "name", Detail: "section " + sec.Name + " already exists in the building"}
	}
	if err := st.InsertSection(ctx, sec); err != nil {
		return domain.Section{}, fmt.Errorf("add section: %w", err)
	}
	return sec, nil
}

// BuildingView is a building with its sections.
type BuildingView struct {
	Building domain.Building
	Sections []domain.Section
}

// ListBuildings returns the buildings of a project with their sections.
func (s *Service) ListBuildings(ctx context.Context, projectID domain.ID) ([]BuildingView, error) {
	buildings, err := s.repo.ListBuildings(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list buildings: %w", err)
	}
	sections, err := s.repo.ListSectionsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list sections: %w", err)
	}
	views := make([]BuildingView, len(buildings))
	index := make(map[domain.ID]int, len(buildings))
	for i, b := range buildings {
		views[i] = BuildingView{Building: b}
		index[b.ID] = i
	}
	for _, sec := range sections {
		if i, ok := index[sec.BuildingID]; ok {
			views[i].Sections = append(views[i].Sections, sec)
		}
	}
	return views, nil
}

// RegisterDataSource registers a data source.
func (s *Service) RegisterDataSource(ctx context.Context, code, name string, kind domain.DataSourceKind) (domain.DataSource, error) {
	ds, err := domain.NewDataSource(s.newID(), code, name, kind, s.clock())
	if err != nil {
		return domain.DataSource{}, err
	}
	existing, err := s.repo.FindDataSourceByCode(ctx, ds.Code)
	if err != nil {
		return domain.DataSource{}, fmt.Errorf("register data source: %w", err)
	}
	if existing != nil {
		return domain.DataSource{}, &domain.RuleError{Code: domain.CodeCodeTaken, Field: "code", Detail: "data source code " + ds.Code + " is taken"}
	}
	if err := s.repo.InsertDataSource(ctx, ds); err != nil {
		return domain.DataSource{}, fmt.Errorf("register data source: %w", err)
	}
	return ds, nil
}

// DataSourceByCode returns a data source by its code.
func (s *Service) DataSourceByCode(ctx context.Context, code string) (domain.DataSource, error) {
	ds, err := s.repo.FindDataSourceByCode(ctx, strings.TrimSpace(code))
	if err != nil {
		return domain.DataSource{}, fmt.Errorf("find data source %q: %w", code, err)
	}
	if ds == nil {
		return domain.DataSource{}, fmt.Errorf("data source %q: %w", code, domain.ErrNotFound)
	}
	return *ds, nil
}

// ListDataSources returns all data sources.
func (s *Service) ListDataSources(ctx context.Context) ([]domain.DataSource, error) {
	return s.repo.ListDataSources(ctx)
}

// AddToPortfolioInput is the input of AddProjectToPortfolio.
type AddToPortfolioInput struct {
	ProjectID domain.ID
	ValidFrom *time.Time
	ValidTo   *time.Time
	Note      string
}

// AddProjectToPortfolio marks a project as a company portfolio project for a period.
func (s *Service) AddProjectToPortfolio(ctx context.Context, in AddToPortfolioInput) (domain.PortfolioMembership, error) {
	var m domain.PortfolioMembership
	err := s.repo.InTx(ctx, func(st Store) error {
		if err := st.LockProject(ctx, in.ProjectID); err != nil {
			return fmt.Errorf("add project %s to portfolio: %w", in.ProjectID, err)
		}
		existing, err := st.ListPortfolioMemberships(ctx, in.ProjectID)
		if err != nil {
			return fmt.Errorf("add project to portfolio: %w", err)
		}
		m, err = domain.NewPortfolioMembership(s.newID(), in.ProjectID, in.ValidFrom, in.ValidTo, in.Note, existing, s.clock())
		if err != nil {
			return err
		}
		return st.InsertPortfolioMembership(ctx, m)
	})
	return m, err
}

// ListPortfolioMemberships returns the portfolio periods of a project.
func (s *Service) ListPortfolioMemberships(ctx context.Context, projectID domain.ID) ([]domain.PortfolioMembership, error) {
	return s.repo.ListPortfolioMemberships(ctx, projectID)
}

// RelationInput is the input of EstablishProjectRelation.
type RelationInput struct {
	SubjectProjectID domain.ID
	RelatedProjectID domain.ID
	Type             domain.RelationType
	Rationale        string
	ValidFrom        time.Time
	ValidTo          *time.Time
}

// EstablishProjectRelation creates a directed relation between two projects.
func (s *Service) EstablishProjectRelation(ctx context.Context, in RelationInput) (domain.ProjectRelation, error) {
	var r domain.ProjectRelation
	err := s.repo.InTx(ctx, func(st Store) error {
		if err := st.LockProject(ctx, in.SubjectProjectID); err != nil {
			return fmt.Errorf("establish relation: subject project %s: %w", in.SubjectProjectID, err)
		}
		if _, err := st.GetProject(ctx, in.RelatedProjectID); err != nil {
			return fmt.Errorf("establish relation: related project %s: %w", in.RelatedProjectID, err)
		}
		existing, err := st.ListRelationsBySubject(ctx, in.SubjectProjectID)
		if err != nil {
			return fmt.Errorf("establish relation: %w", err)
		}
		r, err = domain.NewProjectRelation(s.newID(), in.SubjectProjectID, in.RelatedProjectID, in.Type, in.Rationale, in.ValidFrom, in.ValidTo, existing, s.clock())
		if err != nil {
			return err
		}
		return st.InsertRelation(ctx, r)
	})
	return r, err
}

// RelationView is a relation of a focal project with the related project.
type RelationView struct {
	Relation       domain.ProjectRelation
	RelatedProject domain.Project
	Active         bool
}

// ListProjectRelations returns the relations where the project is the subject
// (its competitors and references), with activity on the business date of the
// given moment.
func (s *Service) ListProjectRelations(ctx context.Context, subjectID domain.ID, on time.Time) ([]RelationView, error) {
	on = domain.BusinessDateOf(on)
	relations, err := s.repo.ListRelationsBySubject(ctx, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list relations of project %s: %w", subjectID, err)
	}
	views := make([]RelationView, 0, len(relations))
	for _, r := range relations {
		related, err := s.repo.GetProject(ctx, r.RelatedProjectID)
		if err != nil {
			return nil, fmt.Errorf("list relations: related project %s: %w", r.RelatedProjectID, err)
		}
		views = append(views, RelationView{Relation: r, RelatedProject: related, Active: r.ActiveOn(on)})
	}
	return views, nil
}

// IncomingRelationView is a relation in which the project is the related one,
// with the subject project that names it.
type IncomingRelationView struct {
	Relation       domain.ProjectRelation
	SubjectProject domain.Project
	Active         bool
}

// ListIncomingProjectRelations returns the relations where the project is the
// related one: the projects for which it is a competitor or a reference. The
// relation stays directed; this is the same record seen from the other side,
// no mirror relation is created (core-domain-model.md, 4.3).
func (s *Service) ListIncomingProjectRelations(ctx context.Context, relatedID domain.ID, on time.Time) ([]IncomingRelationView, error) {
	on = domain.BusinessDateOf(on)
	relations, err := s.repo.ListRelationsByRelated(ctx, relatedID)
	if err != nil {
		return nil, fmt.Errorf("list incoming relations of project %s: %w", relatedID, err)
	}
	views := make([]IncomingRelationView, 0, len(relations))
	for _, r := range relations {
		subject, err := s.repo.GetProject(ctx, r.SubjectProjectID)
		if err != nil {
			return nil, fmt.Errorf("list incoming relations: subject project %s: %w", r.SubjectProjectID, err)
		}
		views = append(views, IncomingRelationView{Relation: r, SubjectProject: subject, Active: r.ActiveOn(on)})
	}
	return views, nil
}

// IsNotFound reports whether err means a missing object.
func IsNotFound(err error) bool { return errors.Is(err, domain.ErrNotFound) }
