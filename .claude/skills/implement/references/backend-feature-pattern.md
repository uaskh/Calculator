# Backend feature pattern

A complete, validated vertical slice (domain → storage adapter → HTTP handler → wiring →
tests) for an illustrative "widget" resource: create a widget, and list widgets in a sort
order chosen from a registry. The names are placeholders: copy the shape, not the domain.
Everything here builds, lints clean and passes `go test -race` (with at least 90% coverage
of the domain package) together with the skeleton in `../templates/backend/` (see
[scaffold.md](scaffold.md)).

## Recipe

1. **Domain** (`internal/<domain>`): types, rules, sentinel and typed errors, a `Service`
   whose dependencies (store, clock, ID generator) are small interfaces or functions. No
   `net/http`, no JSON tags. Table-driven tests with fakes, plus a fuzz test for every
   function that accepts free-form input.
2. **Adapter** (`internal/storage` or `internal/<external>`): implements the domain's
   interface, is safe for concurrent use, and has its own tests (run with `-race`).
3. **HTTP** (`internal/httpapi/<domain>.go`): a consumer-side interface for the service, a
   handler struct, request/response DTOs, `validate()` returning every field error, one
   error-mapping function, route registration in `NewRouter`.
4. **Wiring** (`internal/app`): construct the service and pass it in `httpapi.Deps`.
5. **Contract**: add the operations to `api/openapi.yaml` (schemas, examples, error responses).
6. **Black-box test** in `test/e2e`.
7. Verify: `go vet ./...`, `golangci-lint run ./...`, `go test -race ./...`,
   `go test -run='^$' -fuzz=Fuzz… -fuzztime=30s ./internal/<domain>/`.

**Validation split.** The handler checks the request against the contract and answers 400
with every field error. The domain enforces its rules again; a violation that still reaches
it becomes a 422 in the single error-mapping function.

**Open/closed in practice.** When the spec has a family of similar behaviours (rules,
strategies, formats, sort orders), register the implementations in one table owned by the
domain package and look them up by key, as `NewService` does for sort orders. A new variant
is one entry plus one test row; handlers, DTOs and the error mapping stay untouched, and
unknown keys produce a typed error that lists the allowed values.

### `internal/widget/widget.go`

```go
// Package widget holds the business rules for widgets.
package widget

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxNameLength is the longest allowed widget name, in characters.
const MaxNameLength = 64

// Sort orders accepted by List.
const (
	OrderCreated = "created" // oldest first; the default
	OrderName    = "name"    // alphabetical, ignoring case
)

// ErrInvalidName reports a name that breaks the naming rules.
var ErrInvalidName = errors.New("invalid widget name")

// UnknownOrderError reports a sort order that is not registered.
type UnknownOrderError struct {
	Order   string
	Allowed []string
}

// Error implements the error interface.
func (e *UnknownOrderError) Error() string {
	return fmt.Sprintf("unknown sort order %q (allowed: %s)", e.Order, strings.Join(e.Allowed, ", "))
}

// Widget is a named item.
type Widget struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// Store persists widgets. List returns a slice the caller owns.
type Store interface {
	Save(ctx context.Context, w Widget) error
	List(ctx context.Context) ([]Widget, error)
}

// Service implements the widget use cases.
type Service struct {
	store  Store
	newID  func() string
	now    func() time.Time
	orders map[string]func(a, b Widget) int
}

// NewService returns a Service that persists to store, mints IDs with newID and reads the
// current time from now.
func NewService(store Store, newID func() string, now func() time.Time) *Service {
	return &Service{
		store: store,
		newID: newID,
		now:   now,
		// The single registry of sort orders. A new order is one entry here and one row in
		// TestService_List; ties are broken by ID so that every order is deterministic.
		orders: map[string]func(a, b Widget) int{
			OrderCreated: func(a, b Widget) int {
				return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.ID, b.ID))
			},
			OrderName: func(a, b Widget) int {
				return cmp.Or(
					strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
					strings.Compare(a.ID, b.ID),
				)
			},
		},
	}
}

// ValidateName reports whether name, once trimmed, satisfies the naming rules.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return fmt.Errorf("%w: must be 1 to %d characters", ErrInvalidName, MaxNameLength)
	}
	return nil
}

// Create validates the name and stores a new widget.
func (s *Service) Create(ctx context.Context, name string) (Widget, error) {
	if err := ValidateName(name); err != nil {
		return Widget{}, err
	}
	w := Widget{ID: s.newID(), Name: strings.TrimSpace(name), CreatedAt: s.now().UTC()}
	if err := s.store.Save(ctx, w); err != nil {
		return Widget{}, fmt.Errorf("save widget: %w", err)
	}
	return w, nil
}

// List returns every widget in the given sort order; an empty order means OrderCreated.
func (s *Service) List(ctx context.Context, order string) ([]Widget, error) {
	if order == "" {
		order = OrderCreated
	}
	compare, ok := s.orders[order]
	if !ok {
		return nil, &UnknownOrderError{Order: order, Allowed: s.Orders()}
	}
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list widgets: %w", err)
	}
	slices.SortFunc(items, compare)
	return items, nil
}

// Orders returns the names of the registered sort orders, alphabetically.
func (s *Service) Orders() []string {
	return slices.Sorted(maps.Keys(s.orders))
}
```

### `internal/widget/widget_test.go`

```go
package widget

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var epoch = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

type fakeStore struct {
	items   []Widget
	saveErr error
	listErr error
}

func (f *fakeStore) Save(_ context.Context, w Widget) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.items = append(f.items, w)
	return nil
}

func (f *fakeStore) List(_ context.Context) ([]Widget, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return slices.Clone(f.items), nil
}

func newTestService(store Store) *Service {
	return NewService(store, func() string { return "id-1" }, func() time.Time { return epoch })
}

func sameWidget(a, b Widget) bool {
	return a.ID == b.ID && a.Name == b.Name && a.CreatedAt.Equal(b.CreatedAt)
}

func widgetIDs(items []Widget) []string {
	ids := make([]string, 0, len(items))
	for _, w := range items {
		ids = append(ids, w.ID)
	}
	return ids
}

func TestService_Create(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantName string
		wantErr  error
	}{
		{"stores a trimmed name", "  lamp ", "lamp", nil},
		{"accepts the maximum length", strings.Repeat("é", MaxNameLength), strings.Repeat("é", MaxNameLength), nil},
		{"rejects an empty name", "", "", ErrInvalidName},
		{"rejects whitespace only", " \t ", "", ErrInvalidName},
		{"rejects a name that is too long", strings.Repeat("a", MaxNameLength+1), "", ErrInvalidName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &fakeStore{}

			got, err := newTestService(store).Create(t.Context(), tc.input)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Create(%q) error = %v, want %v", tc.input, err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if len(store.items) != 0 {
					t.Errorf("stored %v after a validation error", store.items)
				}
				return
			}
			want := Widget{ID: "id-1", Name: tc.wantName, CreatedAt: epoch}
			if !sameWidget(got, want) || len(store.items) != 1 || !sameWidget(store.items[0], want) {
				t.Errorf("Create(%q) = %+v, stored %+v; want %+v", tc.input, got, store.items, want)
			}
		})
	}
}

func TestService_Create_StoreFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("disk full")

	if _, err := newTestService(&fakeStore{saveErr: boom}).Create(t.Context(), "lamp"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want wrapped %v", err, boom)
	}
}

func TestService_List(t *testing.T) {
	t.Parallel()
	pear := Widget{ID: "a", Name: "pear", CreatedAt: epoch}
	apple := Widget{ID: "b", Name: "Apple", CreatedAt: epoch.Add(2 * time.Minute)}
	appleToo := Widget{ID: "c", Name: "apple", CreatedAt: epoch.Add(time.Minute)}
	tests := []struct {
		name    string
		order   string
		wantIDs []string
	}{
		{"defaults to creation order", "", []string{"a", "c", "b"}},
		{"creation order", OrderCreated, []string{"a", "c", "b"}},
		{"name order ignores case and breaks ties by ID", OrderName, []string{"b", "c", "a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &fakeStore{items: []Widget{apple, pear, appleToo}}

			got, err := newTestService(store).List(t.Context(), tc.order)

			if err != nil {
				t.Fatalf("List(%q) error = %v", tc.order, err)
			}
			if ids := widgetIDs(got); !slices.Equal(ids, tc.wantIDs) {
				t.Errorf("List(%q) = %v, want %v", tc.order, ids, tc.wantIDs)
			}
		})
	}
}

func TestService_List_EveryOrderIsDeterministic(t *testing.T) {
	t.Parallel()
	items := []Widget{
		{ID: "b", Name: "same", CreatedAt: epoch},
		{ID: "a", Name: "same", CreatedAt: epoch},
		{ID: "c", Name: "Same", CreatedAt: epoch},
	}
	reversed := slices.Clone(items)
	slices.Reverse(reversed)

	for _, order := range newTestService(&fakeStore{}).Orders() {
		first, err := newTestService(&fakeStore{items: items}).List(t.Context(), order)
		if err != nil {
			t.Fatalf("List(%q) error = %v", order, err)
		}
		second, err := newTestService(&fakeStore{items: reversed}).List(t.Context(), order)
		if err != nil {
			t.Fatalf("List(%q) error = %v", order, err)
		}
		if !slices.Equal(widgetIDs(first), widgetIDs(second)) {
			t.Errorf("order %q depends on the input order: %v vs %v", order, widgetIDs(first), widgetIDs(second))
		}
	}
}

func TestService_List_Errors(t *testing.T) {
	t.Parallel()

	_, err := newTestService(&fakeStore{}).List(t.Context(), "size")
	var unknown *UnknownOrderError
	if !errors.As(err, &unknown) || !slices.Equal(unknown.Allowed, []string{OrderCreated, OrderName}) {
		t.Fatalf("List(size) error = %v, want an UnknownOrderError listing the registered orders", err)
	}
	if msg := err.Error(); !strings.Contains(msg, `"size"`) || !strings.Contains(msg, "created, name") {
		t.Errorf("error message = %q", msg)
	}

	boom := errors.New("store offline")
	if _, err := newTestService(&fakeStore{listErr: boom}).List(t.Context(), OrderName); !errors.Is(err, boom) {
		t.Errorf("List error = %v, want wrapped %v", err, boom)
	}
}

func FuzzService_Create(f *testing.F) {
	for _, seed := range []string{"", " ", "lamp", strings.Repeat("a", MaxNameLength+1), "\x00", "💡"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		w, err := newTestService(&fakeStore{}).Create(t.Context(), name)
		if err != nil {
			if !errors.Is(err, ErrInvalidName) || ValidateName(name) == nil {
				t.Fatalf("Create(%q) error = %v", name, err)
			}
			return
		}
		if w.Name == "" || w.Name != strings.TrimSpace(w.Name) || utf8.RuneCountInString(w.Name) > MaxNameLength {
			t.Fatalf("invalid stored name %q", w.Name)
		}
	})
}
```

### `internal/storage/widgets.go`

```go
// Package storage contains persistence adapters for the domain packages.
package storage

import (
	"context"
	"maps"
	"slices"
	"sync"

	"example.com/service/internal/widget"
)

// WidgetStore keeps widgets in process memory. It is safe for concurrent use.
type WidgetStore struct {
	mu    sync.RWMutex
	items map[string]widget.Widget
}

// NewWidgetStore returns an empty in-memory store.
func NewWidgetStore() *WidgetStore {
	return &WidgetStore{items: make(map[string]widget.Widget)}
}

// Save stores w, replacing any widget with the same ID.
func (s *WidgetStore) Save(_ context.Context, w widget.Widget) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[w.ID] = w
	return nil
}

// List returns a copy of every widget, in no particular order.
func (s *WidgetStore) List(_ context.Context) ([]widget.Widget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Collect(maps.Values(s.items)), nil
}
```

### `internal/storage/widgets_test.go`

```go
package storage

import (
	"slices"
	"strconv"
	"sync"
	"testing"

	"example.com/service/internal/widget"
)

func mustList(t *testing.T, s *WidgetStore) []widget.Widget {
	t.Helper()
	items, err := s.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	return items
}

func TestWidgetStore_ConcurrentUse(t *testing.T) {
	t.Parallel()
	const writers = 8
	store := NewWidgetStore()

	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := widget.Widget{ID: "w-" + strconv.Itoa(i), Name: "lamp"}
			if err := store.Save(t.Context(), w); err != nil {
				t.Errorf("Save(%s) error = %v", w.ID, err)
			}
			if _, err := store.List(t.Context()); err != nil {
				t.Errorf("List() error = %v", err)
			}
		}()
	}
	wg.Wait()

	items := mustList(t, store)
	if len(items) != writers {
		t.Fatalf("List() returned %d widgets, want %d", len(items), writers)
	}
	// The caller owns the returned slice: changing it must not change the store.
	items[0].Name = "changed"
	if slices.ContainsFunc(mustList(t, store), func(w widget.Widget) bool { return w.Name == "changed" }) {
		t.Error("List() exposed the store's internal state")
	}
}

func TestWidgetStore_SaveReplacesByID(t *testing.T) {
	t.Parallel()
	store := NewWidgetStore()
	for _, name := range []string{"old", "new"} {
		if err := store.Save(t.Context(), widget.Widget{ID: "w-1", Name: name}); err != nil {
			t.Fatal(err)
		}
	}

	if items := mustList(t, store); len(items) != 1 || items[0].Name != "new" {
		t.Errorf("List() = %+v, want only the replacement", items)
	}
}
```

### `internal/httpapi/widgets.go`

```go
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/service/internal/widget"
)

// WidgetService is what the HTTP layer needs from the widget domain.
type WidgetService interface {
	Create(ctx context.Context, name string) (widget.Widget, error)
	List(ctx context.Context, order string) ([]widget.Widget, error)
}

type widgetHandler struct {
	widgets      WidgetService
	maxBodyBytes int64
}

// createWidgetRequest uses a pointer to tell a missing field from an empty one.
type createWidgetRequest struct {
	Name *string `json:"name"`
}

// validate checks the request against the contract (api/openapi.yaml); every problem is a
// field error in one 400 response. The domain enforces its rules again.
func (r createWidgetRequest) validate() []FieldError {
	var errs []FieldError
	switch {
	case r.Name == nil:
		errs = append(errs, FieldError{Field: "name", Code: "REQUIRED", Message: "is required"})
	case widget.ValidateName(*r.Name) != nil:
		errs = append(errs, FieldError{Field: "name", Code: "INVALID_LENGTH", Message: nameRule()})
	}
	return errs
}

func nameRule() string {
	return "must be 1 to " + strconv.Itoa(widget.MaxNameLength) + " characters"
}

type widgetResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func toWidgetResponse(w widget.Widget) widgetResponse {
	return widgetResponse{ID: w.ID, Name: w.Name, CreatedAt: w.CreatedAt}
}

// listWidgetsResponse wraps the items so that paging fields can be added without a
// breaking change.
type listWidgetsResponse struct {
	Items []widgetResponse `json:"items"`
}

func (h widgetHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createWidgetRequest
	if p := decodeJSON(w, r, h.maxBodyBytes, &req); p != nil {
		writeProblem(w, r, *p)
		return
	}
	if errs := req.validate(); len(errs) > 0 {
		writeProblem(w, r, Problem{
			Status: http.StatusBadRequest,
			Code:   CodeValidationFailed,
			Detail: "The request body is invalid.",
			Errors: errs,
		})
		return
	}
	created, err := h.widgets.Create(r.Context(), *req.Name)
	if err != nil {
		writeWidgetError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/widgets/"+created.ID)
	writeJSON(w, r, http.StatusCreated, toWidgetResponse(created))
}

func (h widgetHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.widgets.List(r.Context(), r.URL.Query().Get("sort"))
	if err != nil {
		writeWidgetError(w, r, err)
		return
	}
	resp := listWidgetsResponse{Items: make([]widgetResponse, 0, len(items))}
	for _, item := range items {
		resp.Items = append(resp.Items, toWidgetResponse(item))
	}
	writeJSON(w, r, http.StatusOK, resp)
}

// writeWidgetError is the single place where widget domain errors become HTTP responses.
func writeWidgetError(w http.ResponseWriter, r *http.Request, err error) {
	var unknownOrder *widget.UnknownOrderError
	switch {
	case errors.As(err, &unknownOrder):
		writeProblem(w, r, Problem{
			Status: http.StatusBadRequest,
			Code:   CodeValidationFailed,
			Detail: "The query parameters are invalid.",
			Errors: []FieldError{{
				Field:   "sort",
				Code:    "UNSUPPORTED_VALUE",
				Message: "must be one of: " + strings.Join(unknownOrder.Allowed, ", "),
			}},
		})
	case errors.Is(err, widget.ErrInvalidName):
		// Defence in depth: validate() normally catches this first.
		writeProblem(w, r, Problem{
			Status: http.StatusUnprocessableEntity,
			Code:   "INVALID_NAME",
			Detail: "The name breaks the naming rules.",
			Errors: []FieldError{{Field: "name", Code: "INVALID_NAME", Message: nameRule()}},
		})
	default:
		loggerFrom(r.Context()).ErrorContext(r.Context(), "widget request failed", "error", err)
		writeProblem(w, r, Problem{Status: http.StatusInternalServerError, Code: CodeInternal})
	}
}
```

### `internal/httpapi/widgets_test.go`

```go
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/service/internal/widget"
)

var widgetTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// fakeWidgets is a hand-written stand-in for the widget service.
type fakeWidgets struct {
	items    []widget.Widget
	err      error
	gotOrder string
}

func (f *fakeWidgets) Create(_ context.Context, name string) (widget.Widget, error) {
	if f.err != nil {
		return widget.Widget{}, f.err
	}
	return widget.Widget{ID: "w-1", Name: name, CreatedAt: widgetTime}, nil
}

func (f *fakeWidgets) List(_ context.Context, order string) ([]widget.Widget, error) {
	f.gotOrder = order
	return f.items, f.err
}

func TestCreateWidget(t *testing.T) {
	t.Parallel()
	tooLong := strings.Repeat("a", widget.MaxNameLength+1)
	tests := []struct {
		name       string
		body       string
		domainErr  error
		wantStatus int
		wantCode   string
		wantBody   string
		wantField  string
		wantLocHdr string
	}{
		{name: "created", body: `{"name":"lamp"}`, wantStatus: http.StatusCreated, wantBody: `{"id":"w-1","name":"lamp","createdAt":"2026-01-02T03:04:05Z"}`, wantLocHdr: "/api/v1/widgets/w-1"},
		{name: "missing name", body: `{}`, wantStatus: http.StatusBadRequest, wantCode: CodeValidationFailed, wantField: "name"},
		{name: "null name", body: `{"name":null}`, wantStatus: http.StatusBadRequest, wantCode: CodeValidationFailed, wantField: "name"},
		{name: "blank name", body: `{"name":"  "}`, wantStatus: http.StatusBadRequest, wantCode: CodeValidationFailed, wantField: "name"},
		{name: "name too long", body: `{"name":"` + tooLong + `"}`, wantStatus: http.StatusBadRequest, wantCode: CodeValidationFailed, wantField: "name"},
		{name: "wrong type", body: `{"name":42}`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest, wantField: "name"},
		{name: "rule violation found by the domain", body: `{"name":"lamp"}`, domainErr: fmt.Errorf("%w: reserved", widget.ErrInvalidName), wantStatus: http.StatusUnprocessableEntity, wantCode: "INVALID_NAME", wantField: "name"},
		{name: "unexpected failure", body: `{"name":"lamp"}`, domainErr: errors.New("db down"), wantStatus: http.StatusInternalServerError, wantCode: CodeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := testDeps()
			d.Widgets = &fakeWidgets{err: tc.domainErr}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/widgets", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			NewRouter(d).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tc.wantBody {
				t.Errorf("body = %s, want %s", rec.Body, tc.wantBody)
			}
			if got := rec.Header().Get("Location"); got != tc.wantLocHdr {
				t.Errorf("Location = %q, want %q", got, tc.wantLocHdr)
			}
			if tc.wantCode == "" {
				return
			}
			p := decodeProblemBody(t, rec)
			if p.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", p.Code, tc.wantCode)
			}
			if tc.wantField != "" && (len(p.Errors) != 1 || p.Errors[0].Field != tc.wantField) {
				t.Errorf("errors = %+v, want one for %q", p.Errors, tc.wantField)
			}
			if tc.wantStatus == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "db down") {
				t.Error("internal error details leaked to the client")
			}
		})
	}
}

func TestListWidgets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		query      string
		items      []widget.Widget
		err        error
		wantStatus int
		wantOrder  string
		wantBody   string
		wantField  string
	}{
		{name: "no widgets", wantStatus: http.StatusOK, wantBody: `{"items":[]}`},
		{
			name:       "widgets in the order the service returns",
			query:      "?sort=name",
			items:      []widget.Widget{{ID: "w-2", Name: "desk", CreatedAt: widgetTime}, {ID: "w-1", Name: "lamp", CreatedAt: widgetTime}},
			wantStatus: http.StatusOK,
			wantOrder:  "name",
			wantBody:   `{"items":[{"id":"w-2","name":"desk","createdAt":"2026-01-02T03:04:05Z"},{"id":"w-1","name":"lamp","createdAt":"2026-01-02T03:04:05Z"}]}`,
		},
		{
			name:       "unknown sort order",
			query:      "?sort=size",
			err:        &widget.UnknownOrderError{Order: "size", Allowed: []string{"created", "name"}},
			wantStatus: http.StatusBadRequest,
			wantOrder:  "size",
			wantField:  "sort",
		},
		{name: "unexpected failure", err: errors.New("db down"), wantStatus: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeWidgets{items: tc.items, err: tc.err}
			d := testDeps()
			d.Widgets = fake
			rec := httptest.NewRecorder()

			NewRouter(d).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/widgets"+tc.query, http.NoBody))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body)
			}
			if fake.gotOrder != tc.wantOrder {
				t.Errorf("order passed to the service = %q, want %q", fake.gotOrder, tc.wantOrder)
			}
			if tc.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tc.wantBody {
				t.Errorf("body = %s, want %s", rec.Body, tc.wantBody)
			}
			if tc.wantField != "" {
				p := decodeProblemBody(t, rec)
				if p.Code != CodeValidationFailed || len(p.Errors) != 1 || p.Errors[0].Field != tc.wantField ||
					!strings.Contains(p.Errors[0].Message, "created, name") {
					t.Errorf("problem = %+v, want a %s field error listing the allowed values", p, tc.wantField)
				}
			}
		})
	}
}
```

### `internal/app/app.go (wiring)`

```go
// Package app is the composition root: it builds the domain services and the HTTP layer
// from configuration and wires them together. Nothing else constructs dependencies.
package app

import (
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"

	"example.com/service/internal/config"
	"example.com/service/internal/httpapi"
	"example.com/service/internal/storage"
	"example.com/service/internal/widget"
)

// New returns the fully wired HTTP handler for the service.
func New(cfg config.Config, logger *slog.Logger) http.Handler {
	widgets := widget.NewService(storage.NewWidgetStore(), rand.Text, time.Now)

	return httpapi.NewRouter(httpapi.Deps{
		Logger:         logger,
		MaxBodyBytes:   cfg.HTTP.MaxBodyBytes,
		RequestTimeout: cfg.HTTP.RequestTimeout,
		AllowedOrigins: cfg.AllowedOrigins,
		Widgets:        widgets,
	})
}
```

### `internal/httpapi/router.go` (changes)

```go
type Deps struct {
	// … existing fields …
	Widgets WidgetService
}

func NewRouter(d Deps) http.Handler {
	// … health routes …
	widgets := widgetHandler{widgets: d.Widgets, maxBodyBytes: d.MaxBodyBytes}
	mux.HandleFunc("GET /api/v1/widgets", widgets.list)
	mux.HandleFunc("POST /api/v1/widgets", widgets.create)
	// … middleware chain unchanged …
}
```

### `test/e2e/api_test.go` (additions; also import `strings`)

```go
func TestCreateWidget(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)

	r := createWidget(t, srv, `{"name":"  lamp "}`)

	id, _ := r.body["id"].(string)
	if r.status != http.StatusCreated || r.body["name"] != "lamp" || id == "" {
		t.Fatalf("POST /api/v1/widgets = %d %v", r.status, r.body)
	}
	if loc := r.header.Get("Location"); loc != "/api/v1/widgets/"+id {
		t.Errorf("Location = %q, want /api/v1/widgets/%s", loc, id)
	}
}

func TestListWidgets(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)
	for _, name := range []string{"lamp", "Desk"} {
		if created := createWidget(t, srv, `{"name":"`+name+`"}`); created.status != http.StatusCreated {
			t.Fatalf("create %s = %d %v", name, created.status, created.body)
		}
	}

	r := send(t, srv, newRequest(t, srv, http.MethodGet, "/api/v1/widgets?sort=name", http.NoBody))

	items, _ := r.body["items"].([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		if w, ok := item.(map[string]any); ok {
			name, _ := w["name"].(string)
			names = append(names, name)
		}
	}
	if r.status != http.StatusOK || strings.Join(names, ",") != "Desk,lamp" {
		t.Errorf("GET /api/v1/widgets?sort=name = %d %v", r.status, r.body)
	}

	bad := send(t, srv, newRequest(t, srv, http.MethodGet, "/api/v1/widgets?sort=size", http.NoBody))
	if bad.status != http.StatusBadRequest || bad.body["code"] != "VALIDATION_FAILED" {
		t.Errorf("GET /api/v1/widgets?sort=size = %d %v", bad.status, bad.body)
	}
}

func createWidget(t *testing.T, srv *httptest.Server, body string) response {
	t.Helper()
	req := newRequest(t, srv, http.MethodPost, "/api/v1/widgets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return send(t, srv, req)
}
```

### `api/openapi.yaml` (operations sketch)

```yaml
  /api/v1/widgets:
    get:
      operationId: listWidgets
      summary: List widgets
      parameters:
        - name: sort
          in: query
          required: false
          schema: { type: string, enum: [created, name], default: created }
      responses:
        "200":
          description: The widgets in the requested order.
          content:
            application/json:
              schema:
                type: object
                required: [items]
                properties:
                  items:
                    type: array
                    items: { $ref: "#/components/schemas/Widget" }
        "400": { $ref: "#/components/responses/BadRequest" }
        "500": { $ref: "#/components/responses/InternalError" }
    post:
      operationId: createWidget
      summary: Create a widget
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              additionalProperties: false
              required: [name]
              properties:
                name: { type: string, minLength: 1, maxLength: 64 }
            example: { name: lamp }
      responses:
        "201":
          description: Created.
          headers:
            Location: { schema: { type: string } }
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Widget" }
        "400": { $ref: "#/components/responses/BadRequest" }
        "413": { $ref: "#/components/responses/PayloadTooLarge" }
        "415": { $ref: "#/components/responses/UnsupportedMediaType" }
        "422": { $ref: "#/components/responses/UnprocessableContent" }
        "500": { $ref: "#/components/responses/InternalError" }

# under components.schemas:
    Widget:
      type: object
      required: [id, name, createdAt]
      properties:
        id: { type: string }
        name: { type: string, minLength: 1, maxLength: 64 }
        createdAt: { type: string, format: date-time }
```
