package main

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
	dbType := reg.DB.DataType

	tx, err := reg.PopConnect()
	if err != nil {
		exitGracefully(err)
	}
	defer tx.Close()

	upBytes, err := templateFS.ReadFile(fmt.Sprintf("templates/migrations/auth_tables.%s.sql", dbType))
	if err != nil {
		exitGracefully(err)
	}

	downBytes := []byte(
		"DROP TABLE IF EXISTS users CASCADE; DROP TABLE IF EXISTS tokens CASCADE; DROP TABLE IF EXISTS remember_tokens;",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = reg.CreatePopMigration(upBytes, downBytes, "auth", "sql")
	if err != nil {
		exitGracefully(err)
	}

	err = reg.RunPopMigrations(tx)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/data/user", reg.RootPath+"/data/user.go")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/data/token", reg.RootPath+"/data/token.go")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/data/remember_token",
		reg.RootPath+"/data/remember_token.go",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/middleware/auth", reg.RootPath+"/middleware/auth.go")
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

	data, err = templateFS.ReadFile("templates/handlers/auth-handlers")
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
		reg.RootPath+"/middleware/auth-token.go",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/mailer/password-reset.html.template",
		reg.RootPath+"/mail/password-reset.html.template",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/mailer/password-reset.plain.template",
		reg.RootPath+"/mail/password-reset.plain.template",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/views/signin.jet", reg.RootPath+"/views/signin.jet")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/views/signup.jet", reg.RootPath+"/views/signup.jet")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/views/forgot.jet", reg.RootPath+"/views/forgot.jet")
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate(
		"templates/views/reset-password.jet",
		reg.RootPath+"/views/reset-password.jet",
	)
	if err != nil {
		exitGracefully(err)
	}

	err = copyFileFromTemplate("templates/routes-auth", reg.RootPath+"/routes-auth.go")
	if err != nil {
		exitGracefully(err)
	}

	routesData, err := os.ReadFile(reg.RootPath + "/routes.go")
	if err != nil {
		exitGracefully(err)
	}

	routesStr := string(routesData)
	routesStr = strings.Replace(routesStr, "a.get(\"/\", a.Handlers.Home)", "a.get(\"/\", a.Handlers.Home)\n\ta.App.Routes.Mount(\"/auth\", a.AuthRoutes())", 1)

	err = os.WriteFile(reg.RootPath+"/routes.go", []byte(routesStr), 0644)
	if err != nil {
		exitGracefully(err)
	}

	modelsData, err := os.ReadFile(reg.RootPath + "/data/models.go")
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

	err = os.WriteFile(reg.RootPath+"/data/models.go", []byte(modelsStr), 0644)
	if err != nil {
		exitGracefully(err)
	}

	err = updateHomeTemplate()
	if err != nil {
		exitGracefully(err)
	}

	color.Yellow("\tRunning go mod tidy...")

	cmd := exec.Command("go", "mod", "tidy")
	err = cmd.Start()
	if err != nil {
		exitGracefully(err)
	}

	color.Yellow("Auth setup completed:")
	color.Yellow(" - Migrations for users, tokens, and remember_tokens have been created and executed.")
	color.Yellow(" - Models for users and tokens have been created.")
	color.Yellow(" - Auth middleware has been created.")
	color.Yellow(" - Auth routes have been created.")
	color.Yellow("")
	color.Yellow("Please ensure to add appropriate middleware to your routes.")

	return nil
}

func updateHomeTemplate() error {
	handlersFile := reg.RootPath + "/handlers/handlers.go"
	if _, err := os.Stat(handlersFile); err == nil {
		handlersData, err := os.ReadFile(handlersFile)
		if err == nil {
			handlersStr := string(handlersData)

			if !strings.Contains(handlersStr, `"github.com/hbarral/regius/render"`) {
				handlersStr = strings.Replace(handlersStr, `"github.com/hbarral/regius"`, "\"github.com/hbarral/regius\"\n\t\"github.com/hbarral/regius/render\"", 1)
			}

			oldHomeFunc := `func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	defer h.App.LoadTime(time.Now())
	err := h.render(w, r, "home", nil, nil)
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}`
			newHomeFunc := `func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
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

	err := h.render(w, r, "home", nil, &render.TemplateData{Data: data})
	if err != nil {
		h.App.ErrorLog.Println("error rendering", err)
	}
}`
			if strings.Contains(handlersStr, oldHomeFunc) {
				handlersStr = strings.Replace(handlersStr, oldHomeFunc, newHomeFunc, 1)
				_ = os.WriteFile(handlersFile, []byte(handlersStr), 0644)
			}
		}
	}

	renderer := os.Getenv("RENDERER")
	if renderer == "" {
		renderer = "jet"
	}

	switch strings.ToLower(renderer) {
	case "jet":
		return updateJetHomeTemplate()
	case "go":
		return nil
	case "templ":
		return nil
	default:
		return nil
	}
}

func updateJetHomeTemplate() error {
	homeFile := reg.RootPath + "/views/home.jet"
	data, err := os.ReadFile(homeFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	content := string(data)
	if strings.Contains(content, "nav class=\"navbar") {
		return nil
	}

	newContent := `{{extends "./layouts/base.jet"}}

{{block browserTitle()}}Welcome{{end}}

{{block css()}}

{{end}}

{{block pageContent()}}
<nav class="navbar navbar-expand-lg navbar-light bg-light">
  <div class="container-fluid">
    <a class="navbar-brand" href="/">Regius</a>
    <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarNav" aria-controls="navbarNav" aria-expanded="false" aria-label="Toggle navigation">
      <span class="navbar-toggler-icon"></span>
    </button>
    <div class="collapse navbar-collapse" id="navbarNav">
      <ul class="navbar-nav ms-auto">
        {{if .IsAuthenticated}}
          <li class="nav-item">
            <span class="nav-link">Welcome, {{.Data["userName"]}}</span>
          </li>
          <li class="nav-item">
            <form method="post" action="/auth/signout" class="d-inline">
              <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
              <button type="submit" class="nav-link btn btn-link border-0" style="text-decoration: none;">Sign Out</button>
            </form>
          </li>
        {{else}}
          <li class="nav-item">
            <a class="nav-link" href="/auth/signin">Sign In</a>
          </li>
          <li class="nav-item">
            <a class="nav-link" href="/auth/signup">Sign Up</a>
          </li>
        {{end}}
      </ul>
    </div>
  </div>
</nav>

<div class="col text-center">
  <div class="d-flex align-items-center justify-content-center mt-5">
    <div>
      <img src="/public/images/regius.png" class="mb-5" style="width: 100px;height:auto;">
      <h1>Regius</h1>
      <hr>
      <small class="text-muted">Go build something real</small>
    </div>
  </div>
</div>
{{end}}

{{block js()}}

{{end}}`

	return os.WriteFile(homeFile, []byte(newContent), 0644)
}
