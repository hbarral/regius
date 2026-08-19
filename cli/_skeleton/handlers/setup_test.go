package handlers

import (
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/CloudyKit/jet/v6"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"github.com/hbarral/regius"
	"github.com/hbarral/regius/i18n"
	"github.com/hbarral/regius/mailer"
	"github.com/hbarral/regius/render"

	"regius-app/locales"
)

var (
	reg          regius.Regius
	testSession  *scs.SessionManager
	testHandlers Handlers
)

func TestMain(m *testing.M) {
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stdout, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	testSession = scs.New()
	testSession.Lifetime = 24 * time.Hour
	testSession.Cookie.Persist = true
	testSession.Cookie.SameSite = http.SameSiteLaxMode
	testSession.Cookie.Secure = true

	views := jet.NewSet(
		jet.NewOSFileSystemLoader("../views"),
		jet.InDevelopmentMode(),
	)

	myRenderer := render.Render{
		RootPath: "../",
		Port:     "4000",
		JetViews: views,
		Session:  testSession,
	}

	reg = regius.Regius{
		AppName:       "regius-app",
		Debug:         true,
		ErrorLog:      errorLog,
		InfoLog:       infoLog,
		RootPath:      "../",
		Routes:        nil,
		Render:        &myRenderer,
		Session:       testSession,
		DB:            regius.Database{},
		JetViews:      views,
		EncryptionKey: reg.RandomString(32),
		Cache:         nil,
		Scheduler:     nil,
		Mail:          mailer.Mail{},
		Server:        regius.Server{},
		I18n: regius.I18nConfig{
			Enabled:          true,
			DefaultLocale:    "en",
			SupportedLocales: []string{"en", "es"},
			CookieName:       "locale",
		},
	}

	testHandlers.App = &reg

	if err := i18n.LoadWithDefault(locales.Content, reg.I18n.DefaultLocale); err != nil {
		log.Fatalf("error loading locales: %v", err)
	}

	os.Exit(m.Run())
}

func getRoutes() http.Handler {
	mux := chi.NewRouter()
	mux.Use(reg.SessionLoad)
	mux.Use(reg.Language(reg.I18n))
	mux.Get("/", testHandlers.Home)
	mux.Get("/set-language/{lang}", testHandlers.SetLanguage)

	fileServer := http.FileServer(http.Dir("./../public/"))
	mux.Handle("/public/*", http.StripPrefix("/public", fileServer))

	return mux
}
