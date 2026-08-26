package regius

import (
	"net/http"
	"os"
	"strconv"

	chi "github.com/go-chi/chi/v5"
	middleware "github.com/go-chi/chi/v5/middleware"
)

func (r *Regius) routes() http.Handler {
	appRoutes := chi.NewRouter()
	appRoutes.Use(r.SessionLoad)
	appRoutes.Use(r.NoSurf)
	maxSize, _ := strconv.ParseInt(os.Getenv("MAX_FILESIZE"), 10, 64)
	appRoutes.Use(r.MaxRequestSize(maxSize))
	appRoutes.Use(r.RequestSanitizer(r.config.requestSanitizer))
	appRoutes.Use(r.CheckForMaintenanceMode)

	r.Routes = appRoutes

	mux := chi.NewRouter()
	mux.Use(r.RequestID(r.config.requestID))
	mux.Use(middleware.RealIP)
	mux.Use(r.Language(r.config.i18n))
	mux.Use(r.IPFilter(r.config.ipFilter))

	if r.config.cors.Enabled {
		mux.Use(r.CORS(r.config.cors))
	}

	mux.Use(r.SecurityHeaders(r.config.securityHeaders))
	if r.Debug {
		mux.Use(middleware.Logger)
	}

	mux.Use(middleware.Recoverer)

	if r.SSE != nil {
		mux.Get("/sse/stream", r.SSE.Handler())
	}

	if r.Scalar.Enabled {
		r.registerScalarRoutes(mux)
	}

	mux.Mount("/", appRoutes)

	r.handler = mux
	return appRoutes
}

func (r *Regius) Handler() http.Handler {
	return r.handler
}
