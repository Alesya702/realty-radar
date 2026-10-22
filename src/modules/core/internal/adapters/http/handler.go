// Package httpadapter renders the Core MVP pages (server-side rendering,
// ADR-0005). Handlers only parse forms, call use cases and render results;
// calculations live in the domain and use cases.
package httpadapter

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"realtyradar/src/modules/core/internal/app"
	"realtyradar/src/modules/core/internal/domain"
	"realtyradar/src/web"
)

//go:embed templates
var templateFS embed.FS

// Handler serves the Core pages. The business time zone (A-07) is the
// domain's: the adapter only formats and parses user input in it.
type Handler struct {
	svc    *app.Service
	logger *slog.Logger
	loc    *time.Location
	pages  *web.Renderer
	now    func() time.Time
}

// Nav returns the navigation links of the Core pages.
func Nav() []web.Link {
	return []web.Link{
		{Title: "Проекты", Href: "/projects"},
		{Title: "Портфель компании", Href: "/portfolio"},
		{Title: "Девелоперы", Href: "/developers"},
		{Title: "Источники данных", Href: "/data-sources"},
	}
}

// New creates the handler. nav is the shell navigation shown on every page.
func New(svc *app.Service, logger *slog.Logger, nav []web.Link) (*Handler, error) {
	h := &Handler{svc: svc, logger: logger, loc: domain.BusinessLocation(), now: time.Now}
	pages, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, err
	}
	h.pages, err = web.NewRenderer(pages, "*.html", h.funcs(), nav)
	if err != nil {
		return nil, fmt.Errorf("core templates: %w", err)
	}
	return h, nil
}

// Register registers the Core routes.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects", h.projects)
	mux.HandleFunc("POST /projects", h.createProject)
	mux.HandleFunc("GET /projects/{id}", h.project)
	mux.HandleFunc("POST /projects/{id}/edit", h.editProject)
	mux.HandleFunc("POST /projects/{id}/developer", h.setDeveloper)
	mux.HandleFunc("POST /projects/{id}/buildings", h.addBuilding)
	mux.HandleFunc("POST /projects/{id}/sections", h.addSection)
	mux.HandleFunc("POST /projects/{id}/lots", h.addLot)
	mux.HandleFunc("POST /projects/{id}/relations", h.addRelation)
	mux.HandleFunc("POST /projects/{id}/portfolio", h.addToPortfolio)
	mux.HandleFunc("GET /lots/{id}", h.lot)
	mux.HandleFunc("POST /lots/{id}/observations", h.recordObservation)
	mux.HandleFunc("GET /portfolio", h.portfolio)
	mux.HandleFunc("GET /developers", h.developers)
	mux.HandleFunc("POST /developers", h.createDeveloper)
	mux.HandleFunc("GET /data-sources", h.dataSources)
	mux.HandleFunc("POST /data-sources", h.createDataSource)
}

// formState carries a failed form back to the page so the user does not lose input.
type formState struct {
	Name   string
	Values url.Values
}

// Value returns a submitted value of the form with the given name, or def.
func (f *formState) Value(form, key, def string) string {
	if f == nil || f.Name != form {
		return def
	}
	return f.Values.Get(key)
}

func (h *Handler) render(w http.ResponseWriter, status int, page, title string, data any, errMsg string) {
	h.pages.Render(w, status, page, web.Page{Title: title, Data: data, Error: errMsg})
}

// fail renders an error page for errors that are not form validation errors.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		h.render(w, http.StatusNotFound, "message.html", "Не найдено", "Объект не найден.", "")
		return
	}
	h.logger.Error("core http handler failed", "method", r.Method, "path", r.URL.Path, "error", err)
	h.render(w, http.StatusInternalServerError, "message.html", "Ошибка", "Внутренняя ошибка. Подробности — в журнале приложения.", "")
}

// userError converts a rule violation into a user message. ok is false for
// other errors, which must go to fail.
func userError(err error) (msg string, ok bool) {
	re, isRule := domain.AsRuleError(err)
	if !isRule {
		return "", false
	}
	return ruleMessage(re), true
}

func (h *Handler) pathID(r *http.Request) (domain.ID, error) {
	id, err := domain.ParseID(r.PathValue("id"))
	if err != nil {
		return domain.ID{}, fmt.Errorf("path id %q: %w", r.PathValue("id"), domain.ErrNotFound)
	}
	return id, nil
}

func redirect(w http.ResponseWriter, r *http.Request, target string) {
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// today returns the current business date. The rule lives in the domain, so a
// caller passing a UTC moment gets the same date as the UI (review M6).
func (h *Handler) today() time.Time { return domain.BusinessDateOf(h.now()) }

// ---- form parsing ------------------------------------------------------------

const (
	dateLayout     = "2006-01-02"
	dateTimeLayout = "2006-01-02T15:04"
)

func formText(r *http.Request, key string) string { return strings.TrimSpace(r.PostFormValue(key)) }

func fieldErr(field, code string) error {
	return &domain.RuleError{Code: code, Field: field, Detail: "invalid " + field}
}

func parseOptionalID(r *http.Request, key string) (*domain.ID, error) {
	v := formText(r, key)
	if v == "" {
		return nil, nil
	}
	id, err := domain.ParseID(v)
	if err != nil {
		return nil, fieldErr(key, domain.CodeInvalidValue)
	}
	return &id, nil
}

func parseRequiredID(r *http.Request, key string) (domain.ID, error) {
	id, err := parseOptionalID(r, key)
	if err != nil {
		return domain.ID{}, err
	}
	if id == nil {
		return domain.ID{}, fieldErr(key, domain.CodeRequired)
	}
	return *id, nil
}

func parseOptionalInt(r *http.Request, key string) (*int, error) {
	v := formText(r, key)
	if v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil, fieldErr(key, domain.CodeInvalidNumber)
	}
	return &n, nil
}

// observedField converts a form input into an observed field: an empty input
// is an explicitly absent value (the form shows current values, so clearing
// an input removes the value).
func observedField[T comparable](r *http.Request, key string, parse func(string) (T, error)) (domain.Field[T], error) {
	v := formText(r, key)
	if v == "" {
		return domain.Absent[T](), nil
	}
	parsed, err := parse(v)
	if err != nil {
		if re, ok := domain.AsRuleError(err); ok {
			return domain.Field[T]{}, fieldErr(key, re.Code)
		}
		return domain.Field[T]{}, fieldErr(key, domain.CodeInvalidValue)
	}
	return domain.Value(parsed), nil
}

func parseIntValue(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fieldErr("", domain.CodeInvalidNumber)
	}
	return n, nil
}

func parseStringValue(s string) (string, error) { return s, nil }

func (h *Handler) parseDateTime(r *http.Request, key string) (time.Time, error) {
	v := formText(r, key)
	if v == "" {
		return time.Time{}, fieldErr(key, domain.CodeRequired)
	}
	t, err := time.ParseInLocation(dateTimeLayout, v, h.loc)
	if err != nil {
		return time.Time{}, fieldErr(key, domain.CodeInvalidValue)
	}
	return t, nil
}

func (h *Handler) parseOptionalDate(r *http.Request, key string) (*time.Time, error) {
	v := formText(r, key)
	if v == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation(dateLayout, v, h.loc)
	if err != nil {
		return nil, fieldErr(key, domain.CodeInvalidValue)
	}
	return &t, nil
}

// parseObservedState reads the lot state inputs shared by the "new lot" and
// "new observation" forms.
func parseObservedState(r *http.Request) (domain.ObservedState, error) {
	var st domain.ObservedState
	var err error
	st.Presence = domain.Presence(formText(r, "presence"))
	if status := formText(r, "status"); status != "" {
		st.Status = domain.Value(domain.LotStatus(status))
	}
	if st.Rooms, err = observedField(r, "rooms", parseIntValue); err != nil {
		return st, err
	}
	if st.AreaTotal, err = observedField(r, "area_total", domain.ParseArea); err != nil {
		return st, err
	}
	if st.Price, err = observedField(r, "price", domain.ParseMoney); err != nil {
		return st, err
	}
	// A checkbox: the form always states it, an unchecked box clears it.
	st.PriceOnRequest = domain.Value(formText(r, "price_on_request") != "")
	if st.PriceBeforeDiscount, err = observedField(r, "price_before_discount", domain.ParseMoney); err != nil {
		return st, err
	}
	if st.Finish, err = observedField(r, "finish", parseStringValue); err != nil {
		return st, err
	}
	return st, nil
}
