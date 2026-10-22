package httpadapter

import (
	"context"
	"net/http"
	"strings"
	"time"

	"realtyradar/src/modules/core/internal/app"
	"realtyradar/src/modules/core/internal/domain"
)

// ---- projects list ---------------------------------------------------------------

type projectsPage struct {
	Projects   []app.ProjectSummary
	Developers []domain.Developer
	Form       *formState
}

func (h *Handler) projects(w http.ResponseWriter, r *http.Request) {
	h.renderProjects(w, r, http.StatusOK, nil, "")
}

func (h *Handler) renderProjects(w http.ResponseWriter, r *http.Request, status int, form *formState, errMsg string) {
	ctx := r.Context()
	projects, err := h.svc.ListProjects(ctx, h.today())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	developers, err := h.svc.ListDevelopers(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, status, "projects.html", "Проекты", projectsPage{Projects: projects, Developers: developers, Form: form}, errMsg)
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	developerID, err := parseOptionalID(r, "developer_id")
	var p domain.Project
	if err == nil {
		p, err = h.svc.CreateProject(r.Context(), app.CreateProjectInput{
			Name: formText(r, "name"), DeveloperID: developerID, City: formText(r, "city"),
		})
	}
	if err != nil {
		if msg, ok := userError(err); ok {
			h.renderProjects(w, r, http.StatusUnprocessableEntity, &formState{Name: "project", Values: r.PostForm}, msg)
			return
		}
		h.fail(w, r, err)
		return
	}
	redirect(w, r, "/projects/"+p.ID.String())
}

func (h *Handler) portfolio(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.ListPortfolioProjects(r.Context(), h.today())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "portfolio.html", "Портфель компании", struct {
		Projects []app.ProjectSummary
		Today    time.Time
	}{projects, h.today()}, "")
}

// ---- developers and data sources -----------------------------------------------------

func (h *Handler) developers(w http.ResponseWriter, r *http.Request) {
	h.renderDevelopers(w, r, http.StatusOK, nil, "")
}

func (h *Handler) renderDevelopers(w http.ResponseWriter, r *http.Request, status int, form *formState, errMsg string) {
	developers, err := h.svc.ListDevelopers(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, status, "developers.html", "Девелоперы", struct {
		Developers []domain.Developer
		Form       *formState
	}{developers, form}, errMsg)
}

func (h *Handler) createDeveloper(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.CreateDeveloper(r.Context(), formText(r, "name")); err != nil {
		if msg, ok := userError(err); ok {
			h.renderDevelopers(w, r, http.StatusUnprocessableEntity, &formState{Name: "developer", Values: r.PostForm}, msg)
			return
		}
		h.fail(w, r, err)
		return
	}
	redirect(w, r, "/developers")
}

func (h *Handler) dataSources(w http.ResponseWriter, r *http.Request) {
	h.renderDataSources(w, r, http.StatusOK, nil, "")
}

func (h *Handler) renderDataSources(w http.ResponseWriter, r *http.Request, status int, form *formState, errMsg string) {
	sources, err := h.svc.ListDataSources(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, status, "data_sources.html", "Источники данных", struct {
		Sources []domain.DataSource
		Kinds   []domain.DataSourceKind
		Form    *formState
	}{sources, []domain.DataSourceKind{domain.DataSourceDocumentChannel, domain.DataSourceManual}, form}, errMsg)
}

func (h *Handler) createDataSource(w http.ResponseWriter, r *http.Request) {
	_, err := h.svc.RegisterDataSource(r.Context(), formText(r, "code"), formText(r, "name"), domain.DataSourceKind(formText(r, "kind")))
	if err != nil {
		if msg, ok := userError(err); ok {
			h.renderDataSources(w, r, http.StatusUnprocessableEntity, &formState{Name: "source", Values: r.PostForm}, msg)
			return
		}
		h.fail(w, r, err)
		return
	}
	redirect(w, r, "/data-sources")
}

// ---- project card -------------------------------------------------------------------

type projectPage struct {
	Project       domain.Project
	Developer     *domain.Developer
	Developers    []domain.Developer
	Buildings     []app.BuildingView
	Lots          []app.LotRow
	Relations     []app.RelationView
	Incoming      []app.IncomingRelationView
	Memberships   []domain.PortfolioMembership
	InPortfolio   bool
	OtherProjects []app.ProjectSummary
	Sources       []domain.DataSource
	Statuses      []domain.LotStatus
	ManualSource  string
	Today         time.Time
	Now           time.Time
	Form          *formState
}

func (h *Handler) project(w http.ResponseWriter, r *http.Request) {
	id, err := h.pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.renderProject(w, r, id, http.StatusOK, nil, "")
}

func (h *Handler) loadProjectPage(ctx context.Context, id domain.ID) (projectPage, error) {
	today := h.today()
	page := projectPage{Today: today, Now: h.now(), Statuses: domain.LotStatuses}
	var err error
	if page.Project, err = h.svc.GetProject(ctx, id); err != nil {
		return page, err
	}
	if page.Project.DeveloperID != nil {
		d, err := h.svc.GetDeveloper(ctx, *page.Project.DeveloperID)
		if err != nil {
			return page, err
		}
		page.Developer = &d
	}
	if page.Developers, err = h.svc.ListDevelopers(ctx); err != nil {
		return page, err
	}
	if page.Buildings, err = h.svc.ListBuildings(ctx, id); err != nil {
		return page, err
	}
	if page.Lots, err = h.svc.ListLots(ctx, id); err != nil {
		return page, err
	}
	if page.Relations, err = h.svc.ListProjectRelations(ctx, id, today); err != nil {
		return page, err
	}
	if page.Incoming, err = h.svc.ListIncomingProjectRelations(ctx, id, today); err != nil {
		return page, err
	}
	if page.Memberships, err = h.svc.ListPortfolioMemberships(ctx, id); err != nil {
		return page, err
	}
	for _, m := range page.Memberships {
		if m.ActiveOn(today) {
			page.InPortfolio = true
		}
	}
	all, err := h.svc.ListProjects(ctx, today)
	if err != nil {
		return page, err
	}
	for _, p := range all {
		if p.Project.ID != id {
			page.OtherProjects = append(page.OtherProjects, p)
		}
	}
	page.Sources, err = h.svc.ListDataSources(ctx)
	page.ManualSource = manualSourceID(page.Sources)
	return page, err
}

func (h *Handler) renderProject(w http.ResponseWriter, r *http.Request, id domain.ID, status int, form *formState, errMsg string) {
	page, err := h.loadProjectPage(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page.Form = form
	h.render(w, status, "project.html", page.Project.Name, page, errMsg)
}

// projectAction runs a project form action and re-renders the card on a rule violation.
func (h *Handler) projectAction(w http.ResponseWriter, r *http.Request, form string, action func(id domain.ID) (string, error)) {
	id, err := h.pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	target, err := action(id)
	if err != nil {
		if msg, ok := userError(err); ok {
			h.renderProject(w, r, id, http.StatusUnprocessableEntity, &formState{Name: form, Values: r.PostForm}, msg)
			return
		}
		h.fail(w, r, err)
		return
	}
	if target == "" {
		target = "/projects/" + id.String()
	}
	redirect(w, r, target)
}

func (h *Handler) editProject(w http.ResponseWriter, r *http.Request) {
	h.projectAction(w, r, "project_edit", func(id domain.ID) (string, error) {
		developerID, err := parseOptionalID(r, "developer_id")
		if err != nil {
			return "", err
		}
		_, err = h.svc.UpdateProject(r.Context(), app.UpdateProjectInput{
			ProjectID: id, Name: formText(r, "name"), DeveloperID: developerID, City: formText(r, "city"),
		})
		return "", err
	})
}

func (h *Handler) setDeveloper(w http.ResponseWriter, r *http.Request) {
	h.projectAction(w, r, "developer", func(id domain.ID) (string, error) {
		developerID, err := parseOptionalID(r, "developer_id")
		if err != nil {
			return "", err
		}
		return "", h.svc.SetProjectDeveloper(r.Context(), id, developerID)
	})
}

func (h *Handler) addBuilding(w http.ResponseWriter, r *http.Request) {
	h.projectAction(w, r, "building", func(id domain.ID) (string, error) {
		_, err := h.svc.AddBuilding(r.Context(), id, formText(r, "name"))
		return "", err
	})
}

func (h *Handler) addSection(w http.ResponseWriter, r *http.Request) {
	h.projectAction(w, r, "section", func(id domain.ID) (string, error) {
		buildingID, err := parseRequiredID(r, "building_id")
		if err != nil {
			return "", err
		}
		b, err := h.svc.ListBuildings(r.Context(), id)
		if err != nil {
			return "", err
		}
		found := false
		for _, v := range b {
			found = found || v.Building.ID == buildingID
		}
		if !found {
			return "", fieldErr("building", domain.CodePlacementMismatch)
		}
		_, err = h.svc.AddSection(r.Context(), buildingID, formText(r, "name"))
		return "", err
	})
}

func (h *Handler) addRelation(w http.ResponseWriter, r *http.Request) {
	h.projectAction(w, r, "relation", func(id domain.ID) (string, error) {
		related, err := parseRequiredID(r, "related_project_id")
		if err != nil {
			return "", err
		}
		from, err := h.parseOptionalDate(r, "valid_from")
		if err != nil {
			return "", err
		}
		if from == nil {
			return "", fieldErr("valid_from", domain.CodeRequired)
		}
		to, err := h.parseOptionalDate(r, "valid_to")
		if err != nil {
			return "", err
		}
		_, err = h.svc.EstablishProjectRelation(r.Context(), app.RelationInput{
			SubjectProjectID: id, RelatedProjectID: related,
			Type: domain.RelationType(formText(r, "relation_type")), Rationale: formText(r, "rationale"),
			ValidFrom: *from, ValidTo: to,
		})
		return "", err
	})
}

func (h *Handler) addToPortfolio(w http.ResponseWriter, r *http.Request) {
	h.projectAction(w, r, "portfolio", func(id domain.ID) (string, error) {
		from, err := h.parseOptionalDate(r, "valid_from")
		if err != nil {
			return "", err
		}
		to, err := h.parseOptionalDate(r, "valid_to")
		if err != nil {
			return "", err
		}
		_, err = h.svc.AddProjectToPortfolio(r.Context(), app.AddToPortfolioInput{ProjectID: id, ValidFrom: from, ValidTo: to, Note: formText(r, "note")})
		return "", err
	})
}

// addLot registers a lot with its first observed state through the ingestion
// contract (origin manual_entry), exactly as a future confirmed import would.
func (h *Handler) addLot(w http.ResponseWriter, r *http.Request) {
	h.projectAction(w, r, "lot", func(id domain.ID) (string, error) {
		batch, err := h.parseBatchHeader(r, id)
		if err != nil {
			return "", err
		}
		placement := &app.NewLotPlacement{Type: domain.LotType(formText(r, "lot_type")), Number: formText(r, "number")}
		if placement.Floor, err = parseOptionalInt(r, "floor"); err != nil {
			return "", err
		}
		switch where := formText(r, "placement"); {
		case strings.HasPrefix(where, "b:"):
			bid, err := domain.ParseID(strings.TrimPrefix(where, "b:"))
			if err != nil {
				return "", fieldErr("building", domain.CodeInvalidValue)
			}
			batch.Buildings = []app.BuildingDecision{{Ref: "building", ExistingID: &bid}}
			placement.BuildingRef = "building"
		case strings.HasPrefix(where, "s:"):
			sid, bid, err := h.sectionPlacement(r.Context(), id, strings.TrimPrefix(where, "s:"))
			if err != nil {
				return "", err
			}
			batch.Buildings = []app.BuildingDecision{{Ref: "building", ExistingID: &bid}}
			batch.Sections = []app.SectionDecision{{Ref: "section", ExistingID: &sid}}
			placement.BuildingRef, placement.SectionRef = "building", "section"
		case where != "":
			return "", fieldErr("building", domain.CodeInvalidValue)
		}
		state, err := parseObservedState(r)
		if err != nil {
			return "", err
		}
		batch.Items = []app.LotObservation{{Ref: "lot", Target: app.LotTarget{NewLot: placement}, State: state,
			SourceKey: optionalString(formText(r, "source_key")), SourceURL: optionalString(formText(r, "source_url"))}}
		res, err := h.svc.IngestObservationBatch(r.Context(), batch)
		if err != nil {
			return "", unwrapItemError(err)
		}
		return "/lots/" + res.Items[0].LotID.String(), nil
	})
}

func (h *Handler) sectionPlacement(ctx context.Context, projectID domain.ID, raw string) (sectionID, buildingID domain.ID, err error) {
	sectionID, err = domain.ParseID(raw)
	if err != nil {
		return sectionID, buildingID, fieldErr("section", domain.CodeInvalidValue)
	}
	buildings, err := h.svc.ListBuildings(ctx, projectID)
	if err != nil {
		return sectionID, buildingID, err
	}
	for _, b := range buildings {
		for _, s := range b.Sections {
			if s.ID == sectionID {
				return sectionID, b.Building.ID, nil
			}
		}
	}
	return sectionID, buildingID, fieldErr("section", domain.CodePlacementMismatch)
}

func (h *Handler) parseBatchHeader(r *http.Request, projectID domain.ID) (app.ObservationBatch, error) {
	observedAt, err := h.parseDateTime(r, "observed_at")
	if err != nil {
		return app.ObservationBatch{}, err
	}
	sourceID, err := parseRequiredID(r, "data_source_id")
	if err != nil {
		return app.ObservationBatch{}, fieldErr("data_source", domain.CodeRequired)
	}
	return app.ObservationBatch{
		DataSourceID: sourceID,
		OriginKind:   domain.OriginManualEntry,
		ObservedAt:   observedAt,
		// The user states the moment the values were seen in the source.
		ObservedAtBasis: domain.BasisEntryTime,
		Completeness:    domain.CompletenessPartial,
		ProjectID:       projectID,
	}, nil
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// unwrapItemError removes the batch item wrapper so that the rule error is shown.
func unwrapItemError(err error) error {
	if re, ok := domain.AsRuleError(err); ok {
		return re
	}
	return err
}

// ---- lot card -----------------------------------------------------------------------

type historyRow struct {
	Version domain.Version
	Changed map[string]bool
}

type lotPage struct {
	History      app.LotHistory
	Project      domain.Project
	Building     string
	Section      string
	Current      *domain.Version
	Rows         []historyRow // newest first
	At           *time.Time
	AtState      *domain.Version
	Sources      []domain.DataSource
	Statuses     []domain.LotStatus
	ManualSource string
	Now          time.Time
	Form         *formState
}

func (h *Handler) lot(w http.ResponseWriter, r *http.Request) {
	id, err := h.pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var at *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("at")); raw != "" {
		t, err := time.ParseInLocation(dateTimeLayout, raw, h.loc)
		if err != nil {
			h.renderLot(w, r, id, http.StatusUnprocessableEntity, nil, nil, ruleMessage(&domain.RuleError{Code: domain.CodeInvalidValue, Field: "at"}))
			return
		}
		at = &t
	}
	h.renderLot(w, r, id, http.StatusOK, at, nil, "")
}

func (h *Handler) renderLot(w http.ResponseWriter, r *http.Request, id domain.ID, status int, at *time.Time, form *formState, errMsg string) {
	ctx := r.Context()
	hist, err := h.svc.GetLotHistory(ctx, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page := lotPage{History: hist, Current: hist.Current(), At: at, Now: h.now(), Form: form, Statuses: domain.LotStatuses}
	if page.Project, err = h.svc.GetProject(ctx, hist.Lot.Placement.ProjectID); err != nil {
		h.fail(w, r, err)
		return
	}
	buildings, err := h.svc.ListBuildings(ctx, page.Project.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, b := range buildings {
		if hist.Lot.Placement.BuildingID != nil && b.Building.ID == *hist.Lot.Placement.BuildingID {
			page.Building = b.Building.Name
			for _, s := range b.Sections {
				if hist.Lot.Placement.SectionID != nil && s.ID == *hist.Lot.Placement.SectionID {
					page.Section = s.Name
				}
			}
		}
	}
	if at != nil {
		page.AtState = hist.At(*at)
	}
	for i := len(hist.Versions) - 1; i >= 0; i-- {
		row := historyRow{Version: hist.Versions[i]}
		if i > 0 {
			row.Changed = changedFields(hist.Versions[i-1].State, hist.Versions[i].State)
		}
		page.Rows = append(page.Rows, row)
	}
	if page.Sources, err = h.svc.ListDataSources(ctx); err != nil {
		h.fail(w, r, err)
		return
	}
	page.ManualSource = manualSourceID(page.Sources)
	title := "Лот " + lotNumberLabel(hist.Lot.Placement.Number)
	h.render(w, status, "lot.html", title, page, errMsg)
}

// changedFields marks the values that differ between two consecutive versions
// (presentation only).
func changedFields(prev, cur domain.LotState) map[string]bool {
	return map[string]bool{
		"presence":              prev.Presence != cur.Presence,
		"status":                prev.Status != cur.Status,
		"rooms":                 !equalPtr(prev.Rooms, cur.Rooms),
		"area_total":            !equalPtr(prev.AreaTotal, cur.AreaTotal),
		"price":                 !equalPtr(prev.Price, cur.Price) || prev.PriceOnRequest != cur.PriceOnRequest,
		"price_before_discount": !equalPtr(prev.PriceBeforeDiscount, cur.PriceBeforeDiscount),
		"finish":                !equalPtr(prev.Finish, cur.Finish),
	}
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (h *Handler) recordObservation(w http.ResponseWriter, r *http.Request) {
	id, err := h.pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	err = func() error {
		lot, err := h.svc.GetLot(r.Context(), id)
		if err != nil {
			return err
		}
		batch, err := h.parseBatchHeader(r, lot.Placement.ProjectID)
		if err != nil {
			return err
		}
		state, err := parseObservedState(r)
		if err != nil {
			return err
		}
		batch.Items = []app.LotObservation{{Ref: "lot", Target: app.LotTarget{ExistingLotID: &id}, State: state, SourceURL: optionalString(formText(r, "source_url"))}}
		_, err = h.svc.IngestObservationBatch(r.Context(), batch)
		return unwrapItemError(err)
	}()
	if err != nil {
		if msg, ok := userError(err); ok {
			h.renderLot(w, r, id, http.StatusUnprocessableEntity, nil, &formState{Name: "observation", Values: r.PostForm}, msg)
			return
		}
		h.fail(w, r, err)
		return
	}
	redirect(w, r, "/lots/"+id.String())
}

// manualSourceID returns the id of the manual entry data source, preselected in forms.
func manualSourceID(sources []domain.DataSource) string {
	for _, s := range sources {
		if s.Code == domain.ManualDataSourceCode {
			return s.ID.String()
		}
	}
	return ""
}
