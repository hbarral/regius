package regius

import (
	"net/http"
	"os"
	"strconv"

	chi "github.com/go-chi/chi/v5"
	middleware "github.com/go-chi/chi/v5/middleware"
)

func (r *Regius) routes() http.Handler {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)

	if r.Debug {
		mux.Use(middleware.Logger)
	}

	mux.Use(middleware.Recoverer)
	mux.Use(r.SessionLoad)
	mux.Use(r.NoSurf)
	maxSize, _ := strconv.ParseInt(os.Getenv("MAX_FILESIZE"), 10, 64)
	mux.Use(r.MaxRequestSize(maxSize))
	mux.Use(r.CheckForMaintenanceMode)

	return mux
}
