package cli

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"
)

func doAuth() error {
	checkForDB()
	appName := os.Getenv("APP_NAME")
	appName = strings.ToLower(appName)
	log.Println("APP NAME IS:", appName)
	dbType := normalizeDBType(b.DBType)
	renderer := strings.ToLower(resolveRenderer())
	if !validRenderers[renderer] {
		log.Println("invalid renderer", renderer, "defaulting to", defaultRenderer)
		renderer = defaultRenderer
	}

	upBytes, err := templateFS.ReadFile(fmt.Sprintf("templates/migrations/auth_tables.%s.sql", dbType))
	if err != nil {
		exitGracefully(err)
	}

	// Drop dependents first so this runs portably on postgres, mysql and
	// sqlite (sqlite rejects the CASCADE keyword on DROP TABLE).
	downBytes := []byte(
		"DROP TABLE IF EXISTS tokens; DROP TABLE IF EXISTS remember_tokens; DROP TABLE IF EXISTS users;",
	)

	if err := b.CreateMigration(upBytes, downBytes, "auth", "sql"); err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/data/user", b.RootPath+"/data/user.go")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/data/token", b.RootPath+"/data/token.go")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/data/remember_token",
		b.RootPath+"/data/remember_token.go",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/middleware/auth", b.RootPath+"/middleware/auth.go")
	if err != nil {
		exitGracefully(err)
	}

	data, err := templateFS.ReadFile("templates/middleware/remember")
	if err != nil {
		exitGracefully(err)
	}

	rememberTokenFileContent := string(data)
	rememberTokenFileContent = strings.ReplaceAll(rememberTokenFileContent, "${APP_NAME}", appName)

	err = copyDataToFile([]byte(rememberTokenFileContent), "./middleware/remember.go")
	if err != nil {
		exitGracefully(err)
	}

	// auth-handlers: pick the variant for the active renderer.
	// Overwrite (not skip) so `make auth --renderer X` can switch engines.
	data, err = templateFS.ReadFile(fmt.Sprintf("templates/handlers/auth-handlers.%s", handlerTemplateSuffix(renderer)))
	if err != nil {
		exitGracefully(err)
	}
	authHandlerFileContent := string(data)
	authHandlerFileContent = strings.ReplaceAll(authHandlerFileContent, "${APP_NAME}", appName)
	err = copyDataToFile([]byte(authHandlerFileContent), "./handlers/auth-handlers.go")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/middleware/auth-token",
		b.RootPath+"/middleware/auth-token.go",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/mailer/password-reset.html.template",
		b.RootPath+"/mail/password-reset.html.template",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/mailer/password-reset.plain.template",
		b.RootPath+"/mail/password-reset.plain.template",
	)
	if err != nil {
		exitGracefully(err)
	}

	// install the auth views for the active renderer (skips any that exist).
	if err := installAuthViews(renderer); err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/routes-auth", b.RootPath+"/routes-auth.go")
	if err != nil {
		exitGracefully(err)
	}

	routesData, err := os.ReadFile(b.RootPath + "/routes.go")
	if err != nil {
		exitGracefully(err)
	}

	routesStr := string(routesData)
	if !strings.Contains(routesStr, "a.AuthRoutes()") {
		routesStr = strings.Replace(routesStr, "a.get(\"/\", a.Handlers.Home)", "a.get(\"/\", a.Handlers.Home)\n\ta.App.Routes.Mount(\"/auth\", a.AuthRoutes())", 1)
	}
	// enable the remember-me middleware.
	routesStr = strings.Replace(routesStr, "// a.use(a.Middleware.CheckRemember)", "a.use(a.Middleware.CheckRemember)", 1)
	err = os.WriteFile(b.RootPath+"/routes.go", []byte(routesStr), 0644)
	if err != nil {
		exitGracefully(err)
	}

	modelsData, err := os.ReadFile(b.RootPath + "/data/models.go")
	if err != nil {
		exitGracefully(err)
	}

	modelsStr := string(modelsData)
	modelsStr = strings.Replace(modelsStr, "// RememberToken RememberToken", "RememberToken RememberToken", 1)
	modelsStr = strings.Replace(modelsStr, "// Users         User", "Users         User", 1)
	modelsStr = strings.Replace(modelsStr, "// Tokens        Token", "Tokens        Token", 1)
	modelsStr = strings.Replace(modelsStr, "// RememberToken: RememberToken{},", "RememberToken: RememberToken{},", 1)
	modelsStr = strings.Replace(modelsStr, "// Users:         User{},", "Users:         User{},", 1)
	modelsStr = strings.Replace(modelsStr, "// Tokens:        Token{},", "Tokens:        Token{},", 1)

	err = os.WriteFile(b.RootPath+"/data/models.go", []byte(modelsStr), 0644)
	if err != nil {
		exitGracefully(err)
	}

	err = updateHomeTemplate(renderer)
	if err != nil {
		exitGracefully(err)
	}

	// regen templ sources if needed.
	if renderer == "templ" {
		if err := runTemplGenerate(); err != nil {
			color.Yellow("  ! templ generate failed; run `templ generate` manually: %v", err)
		}
	}

	color.Yellow("\tRunning go mod tidy...")

	cmd := exec.Command("go", "mod", "tidy")
	err = cmd.Start()
	if err != nil {
		exitGracefully(err)
	}

	color.Yellow("Auth setup completed (renderer=%s):", renderer)
	color.Yellow(" - Migrations for users, tokens, and remember_tokens have been created (run `regius migrate` to apply them).")
	color.Yellow(" - Models for users and tokens have been created.")
	color.Yellow(" - Auth middleware has been created.")
	color.Yellow(" - Auth routes have been created.")
	color.Yellow("")
	color.Yellow("Please ensure to add appropriate middleware to your routes.")

	return nil
}

// updateHomeTemplate patches the scaffolded Home handler and home view to be
// auth-aware for the given renderer. It is idempotent: a view that already has
// a navbar/Sign Out is left untouched.
func updateHomeTemplate(renderer string) error {
	handlersFile := b.RootPath + "/handlers/handlers.go"
	if data, err := os.ReadFile(handlersFile); err == nil {
		handlersStr := string(data)

		if !strings.Contains(handlersStr, `"github.com/hbarral/regius/render"`) {
			handlersStr = strings.Replace(handlersStr, `"github.com/hbarral/regius"`, "\"github.com/hbarral/regius\"\n\t\"github.com/hbarral/regius/render\"", 1)
		}

		// The lean skeleton ships a simple Home. After `make auth`, upgrade it
		// to read the authenticated user's name from the session and pass it
		// to the view.
		var oldHome, newHome string
		switch strings.ToLower(renderer) {
		case "jet", "go":
			// Non-templ lean skeletons don't import `views` in Home by default;
			// they use Render.Jet/Render.Go directly.
			oldHome = `func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	defer h.App.LoadTime(time.Now())
	err := h.App.Render.Page(w, r, h.App.Render.` + renderCallFor(renderer) + `, nil)
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}`
			newHome = `func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	defer h.App.LoadTime(time.Now())

	var userName string
	if h.App.Session.Exists(r.Context(), "userID") {
		userID := h.App.Session.GetInt(r.Context(), "userID")
		u, err := h.Models.Users.Get(userID)
		if err == nil {
			userName = u.FirstName
		}
	}

	data := make(map[string]interface{})
	data["userName"] = userName

	err := h.App.Render.Page(w, r, h.App.Render.` + renderCallFor(renderer) + `, &render.TemplateData{Data: data})
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}`
		default: // templ
			oldHome = `func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	defer h.App.LoadTime(time.Now())
	err := h.App.Render.Page(w, r, views.Home(), nil)
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}`
			newHome = `func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	defer h.App.LoadTime(time.Now())

	var userName string
	if h.App.Session.Exists(r.Context(), "userID") {
		userID := h.App.Session.GetInt(r.Context(), "userID")
		u, err := h.Models.Users.Get(userID)
		if err == nil {
			userName = u.FirstName
		}
	}

	data := make(map[string]interface{})
	data["userName"] = userName

	err := h.App.Render.Page(w, r, views.Home(), &render.TemplateData{Data: data})
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}`
		}

		if strings.Contains(handlersStr, oldHome) && !strings.Contains(handlersStr, "userName") {
			handlersStr = strings.Replace(handlersStr, oldHome, newHome, 1)
			_ = os.WriteFile(handlersFile, []byte(handlersStr), 0644)
		}
	}

	switch strings.ToLower(renderer) {
	case "jet":
		return updateJetHomeTemplate()
	case "go":
		return updateGoHomeTemplate()
	default:
		// templ: the navbar lives in the shared BaseLayout component, so no
		// home template needs patching.
		return nil
	}
}

// renderCallFor returns the Render engine call expression for the given renderer
// as it appears in the Home handler.
func renderCallFor(renderer string) string {
	switch strings.ToLower(renderer) {
	case "jet":
		return `Jet("home", nil)`
	case "go":
		return `GoLayout("home", "base")`
	default:
		return ``
	}
}

func updateJetHomeTemplate() error {
	homeFile := b.RootPath + "/views/home.jet"
	data, err := os.ReadFile(homeFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// The modern jet skeleton keeps the navbar in a shared component, so the
	// home template only needs the centered hero. If it already uses the
	// modern classes, leave it alone.
	content := string(data)
	if strings.Contains(content, "bg-border") && strings.Contains(content, "text-muted-foreground") {
		return nil
	}

	newContent := `{{extends "./layouts/base.jet"}}

{{block browserTitle()}}Welcome{{end}}

{{block css()}}{{end}}

{{block pageContent()}}
<div class="flex h-full items-center justify-center">
  <div class="flex flex-col items-center gap-6 text-center">
    <img src="/public/images/regius.png" alt="Regius" class="h-auto w-24 select-none">
    <h1 class="text-4xl font-bold tracking-tight select-none">
      Regius
    </h1>
    <div class="h-px w-48 bg-border"></div>
    <small class="text-muted-foreground select-none">
      Go build something real
    </small>
  </div>
</div>
{{end}}

{{block js()}}{{end}}`

	return os.WriteFile(homeFile, []byte(newContent), 0644)
}

// updateGoHomeTemplate writes the auth-aware home.page.template for the go
// engine if it does not already contain the navbar.
func updateGoHomeTemplate() error {
	homeFile := b.RootPath + "/views/home.page.template"
	data, err := os.ReadFile(homeFile)
	if err == nil && strings.Contains(string(data), "Sign Out") {
		return nil
	}
	return copyFileFromTemplate("templates/views/home.page.template", homeFile)
}
