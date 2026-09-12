package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const serviceMainFixture = `package main

import (
	"sync"

	"github.com/hbarral/regius"

	"testapp/data"
	"testapp/handlers"
	"testapp/middleware"
)

type application struct {
	App        *regius.Regius
	Handlers   *handlers.Handlers
	Models     data.Models
	Middleware *middleware.Middleware
	wg         sync.WaitGroup
	done       chan struct{}
}
`

const serviceHandlersFixture = `package handlers

import (
	"net/http"

	"github.com/hbarral/regius"

	"testapp/data"
)

type Handlers struct {
	App    *regius.Regius
	Models data.Models
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	h.App.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
`

const serviceInitFixture = `package main

import (
	"github.com/hbarral/regius"

	"testapp/data"
	"testapp/handlers"
	"testapp/middleware"
)

func initApplication() *application {
	reg := &regius.Regius{}

	myMiddleware := &middleware.Middleware{
		App: reg,
	}

	myHandlers := &handlers.Handlers{
		App: reg,
	}

	app := &application{
		App:        reg,
		Handlers:   myHandlers,
		Middleware: myMiddleware,
	}

	app.Models = data.New(nil)
	myHandlers.Models = app.Models
	app.Middleware.Models = app.Models

	// register services here

	// register background workers here

	return app
}
`

// withServiceRoot points the CLI's global backend at a fresh temp dir
// seeded with main.go, handlers/handlers.go, and init.regius.go fixtures,
// with APP_NAME=testapp in the process env.
func withServiceRoot(t *testing.T, initFixture string) string {
	t.Helper()

	old := b.RootPath
	dir := t.TempDir()
	b.RootPath = dir
	t.Setenv("APP_NAME", "testapp")
	t.Cleanup(func() { b.RootPath = old })

	for path, content := range map[string]string{
		"main.go":              serviceMainFixture,
		"handlers/handlers.go": serviceHandlersFixture,
		"init.regius.go":       initFixture,
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func readServiceFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}

func TestDoMakeService_FirstRunBootstraps(t *testing.T) {
	dir := withServiceRoot(t, serviceInitFixture)

	if err := doMakeService("billing"); err != nil {
		t.Fatalf("doMakeService: %v", err)
	}

	// the service file, with every placeholder replaced
	svc := readServiceFile(t, filepath.Join(dir, "services", "billing.go"))
	for _, want := range []string{
		"// BillingService TODO: describe the service's responsibility.",
		"type BillingService struct {",
		"func NewBillingService(app *regius.Regius, models data.Models) *BillingService",
		"func (s *BillingService) Do(ctx context.Context) error",
		"\"testapp/data\"",
	} {
		if !strings.Contains(svc, want) {
			t.Errorf("generated service missing %q:\n%s", want, svc)
		}
	}
	if strings.Contains(svc, "$") {
		t.Errorf("generated service still contains unreplaced placeholder:\n%s", svc)
	}

	// the hub carries the first service
	hub := readServiceFile(t, filepath.Join(dir, "services", "services.go"))
	for _, want := range []string{
		"type Services struct {",
		"// additional service fields are registered here",
		"// additional service constructors are registered here",
		"Billing *BillingService",
		"Billing: NewBillingService(app, models),",
	} {
		if !strings.Contains(hub, want) {
			t.Errorf("services hub missing %q:\n%s", want, hub)
		}
	}

	// main.go: import + Services field on the application struct
	mainGo := readServiceFile(t, filepath.Join(dir, "main.go"))
	if !strings.Contains(mainGo, "\"testapp/services\"") {
		t.Errorf("main.go missing services import:\n%s", mainGo)
	}
	if !strings.Contains(mainGo, "Services   *services.Services") {
		t.Errorf("main.go missing Services field:\n%s", mainGo)
	}

	// handlers/handlers.go: import + Services field
	handlersGo := readServiceFile(t, filepath.Join(dir, "handlers", "handlers.go"))
	if !strings.Contains(handlersGo, "\"testapp/services\"") {
		t.Errorf("handlers.go missing services import:\n%s", handlersGo)
	}
	if !strings.Contains(handlersGo, "Services *services.Services") {
		t.Errorf("handlers.go missing Services field:\n%s", handlersGo)
	}

	// init.regius.go: import + NewServices call at the marker
	initGo := readServiceFile(t, filepath.Join(dir, "init.regius.go"))
	if !strings.Contains(initGo, "\"testapp/services\"") {
		t.Errorf("init.regius.go missing services import:\n%s", initGo)
	}
	if !strings.Contains(initGo, "app.Services = services.NewServices(app.App, app.Models)") {
		t.Errorf("init.regius.go missing NewServices call:\n%s", initGo)
	}
	if !strings.Contains(initGo, "myHandlers.Services = app.Services") {
		t.Errorf("init.regius.go missing handlers injection:\n%s", initGo)
	}
	if strings.Index(initGo, "app.Services = services.NewServices") > strings.Index(initGo, "// register background workers here") {
		t.Errorf("NewServices call should sit at the services marker, before the workers marker:\n%s", initGo)
	}
}

func TestDoMakeService_SecondServiceAppends(t *testing.T) {
	dir := withServiceRoot(t, serviceInitFixture)

	if err := doMakeService("billing"); err != nil {
		t.Fatalf("first doMakeService: %v", err)
	}
	if err := doMakeService("user-account"); err != nil {
		t.Fatalf("second doMakeService: %v", err)
	}

	hub := readServiceFile(t, filepath.Join(dir, "services", "services.go"))
	// gofmt aligns struct fields, so compare with squashed whitespace
	squashed := strings.Join(strings.Fields(hub), " ")
	for _, want := range []string{
		"Billing *BillingService",
		"UserAccount *UserAccountService",
		"Billing: NewBillingService(app, models),",
		"UserAccount: NewUserAccountService(app, models),",
	} {
		if !strings.Contains(squashed, want) {
			t.Errorf("services hub missing %q:\n%s", want, hub)
		}
	}
	// markers stay last: the fields marker directly precedes the struct's
	// closing brace, with both fields above it
	if !strings.Contains(hub, "// additional service fields are registered here\n}") {
		t.Errorf("fields marker is not last:\n%s", hub)
	}
	if strings.Index(hub, "UserAccount *UserAccountService") > strings.Index(hub, "// additional service fields are registered here") {
		t.Errorf("appended field landed below the marker:\n%s", hub)
	}

	// wiring is not duplicated
	initGo := readServiceFile(t, filepath.Join(dir, "init.regius.go"))
	if strings.Count(initGo, "services.NewServices(app.App, app.Models)") != 1 {
		t.Errorf("NewServices call duplicated:\n%s", initGo)
	}
	mainGo := readServiceFile(t, filepath.Join(dir, "main.go"))
	if strings.Count(mainGo, "Services   *services.Services") != 1 {
		t.Errorf("Services field duplicated in main.go:\n%s", mainGo)
	}
}

func TestDoMakeService_ServiceSuffixNotDuplicated(t *testing.T) {
	dir := withServiceRoot(t, serviceInitFixture)

	if err := doMakeService("billing-service"); err != nil {
		t.Fatalf("doMakeService: %v", err)
	}

	svc := readServiceFile(t, filepath.Join(dir, "services", "billing_service.go"))
	if !strings.Contains(svc, "type BillingService struct {") {
		t.Errorf("expected BillingService type, got:\n%s", svc)
	}
	if strings.Contains(svc, "BillingServiceService") {
		t.Errorf("service suffix was duplicated:\n%s", svc)
	}
}

func TestDoMakeService_InitFallbackWithoutMarkers(t *testing.T) {
	oldInit := strings.Replace(serviceInitFixture, "\t// register services here\n\n\t// register background workers here\n", "", 1)
	dir := withServiceRoot(t, oldInit)

	if err := doMakeService("billing"); err != nil {
		t.Fatalf("doMakeService: %v", err)
	}

	initGo := readServiceFile(t, filepath.Join(dir, "init.regius.go"))
	if !strings.Contains(initGo, "app.Services = services.NewServices(app.App, app.Models)") {
		t.Errorf("init.regius.go missing NewServices call (fallback):\n%s", initGo)
	}
	if strings.Index(initGo, "app.Services = services.NewServices") > strings.Index(initGo, "return app") {
		t.Errorf("NewServices call should sit before return app:\n%s", initGo)
	}
}

func TestDoMakeService_DuplicateName(t *testing.T) {
	withServiceRoot(t, serviceInitFixture)

	if err := doMakeService("billing"); err != nil {
		t.Fatalf("first doMakeService: %v", err)
	}
	err := doMakeService("billing")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}

func TestDoMakeService_MissingAppName(t *testing.T) {
	withServiceRoot(t, serviceInitFixture)
	t.Setenv("APP_NAME", "")

	if err := doMakeService("billing"); err == nil || !strings.Contains(err.Error(), "APP_NAME") {
		t.Fatalf("expected APP_NAME error, got %v", err)
	}
}
