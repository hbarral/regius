package regius

import (
	"net/http"

	"github.com/go-chi/cors"
)

// CORSConfig holds configuration for Cross-Origin Resource Sharing.
type CORSConfig struct {
	Enabled            bool
	AllowedOrigins     []string
	AllowedMethods     []string
	AllowedHeaders     []string
	ExposedHeaders     []string
	MaxAge             int
	AllowCredentials   bool
	OptionsPassthrough bool
	Debug              bool
}

// CORS returns a middleware handler that enables CORS with the given configuration.
// When Enabled is false, it returns a no-op passthrough handler.
func (r *Regius) CORS(config CORSConfig) func(next http.Handler) http.Handler {
	if !config.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	opts := cors.Options{
		AllowedOrigins:     config.AllowedOrigins,
		AllowedMethods:     config.AllowedMethods,
		AllowedHeaders:     config.AllowedHeaders,
		ExposedHeaders:     config.ExposedHeaders,
		MaxAge:             config.MaxAge,
		AllowCredentials:   config.AllowCredentials,
		OptionsPassthrough: config.OptionsPassthrough,
		Debug:              config.Debug,
	}

	return cors.Handler(opts)
}
