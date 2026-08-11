# Regius

<p align="center">
  <img src="regius.png" alt="Regius Logo" width="40%">
</p>

Regius is a CLI application for building web pages, inspired by Laravel but built with Go. It offers tools for database migrations and code generation, providing an agile and organized development experience.

## 🌍 Repository

Visit the official repository at [Regius on GitHub](https://github.com/hbarral/regius).

## 📋 Features

### Basic Commands

- `regius new <app_name>`: Creates a new web application (defaults to `templ` renderer + `sqlite` database, with a runnable auth scaffold; switch with `--renderer jet|go` and/or `--db postgres|mysql|...`).
- `regius version`: Print application version.
- `regius help`: Show help for any command.
- `regius up`: Bring the server back from maintenance mode.
- `regius down`: Put the server in maintenance mode.

### Migration Commands

- `regius migrate`: Run all pending migrations (defaults to "up").
- `regius migrate up`: Run all pending migrations.
- `regius migrate down [steps|all]`: Reverse migrations (use "all" for all migrations).
- `regius migrate reset`: Reset and re-run all migrations.
- `regius migrate version`: Show the current migration version (uses `golang-migrate`).

### Seeding

- `regius make seed <name>`: Create a new SQL seed file in `seeds/`.
- `regius db:seed`: Run all pending seed files in `seeds/` (filename order), tracking executed seeds in the `regius_seeds` table.

Seed files are plain `.sql` files executed in a single transaction, and each is only applied once.

### Code Generation Commands

- `regius make migration <name>`: Create SQL migration files.
- `regius make auth`: Create authentication system (tables, models, middleware, handlers, views).
- `regius make handler <name>`: Create a handler stub.
- `regius make model <name>`: Create a new model with proper pluralization.
- `regius make gorm-model <name>`: Create a new GORM model with proper pluralization.
- `regius make session`: Create session table in database.
- `regius make key`: Generate 32-character encryption key.
- `regius make mail <name>`: Create mail templates.
- `regius make locale <code>`: Create a new translation locale file (e.g. `regius make locale fr`).

### CLI Features

- **Automatic help**: `--help` flag on all commands and subcommands
- **Flag support**: Use `--format=sql` for migration format
- **Shell completion**: Generate autocompletion scripts for bash, zsh, fish, and PowerShell
- **Better validation**: Improved argument validation and error messages
- **Command aliases**: Future support for command shortcuts

### Database Features

Regius includes a unified database layer that works out of the box with PostgreSQL, MySQL/MariaDB, and SQLite.

- **Driver alias normalization**: `postgres`/`postgresql`, `mysql`/`mariadb`, and `sqlite`/`sqlite3` are all accepted as `DATABASE_TYPE`.
- **Multi-database DSN builder**: `BuildDSN()` produces the correct DSN for each driver without manual string concatenation.
- **Connection pool tuning**: Configure max open, max idle, and connection lifetime via environment variables.
- **Health checks**: `Database.HealthCheck(ctx)` verifies the database is reachable and responds to a ping.
- **Transaction helper**: `Regius.Transaction(ctx, func(*sql.Tx) error)` runs a block inside a transaction and handles commit/rollback automatically.
- **Read/write splitting**: Configure a read replica via `DATABASE_READ_*` environment variables (or `DATABASE_READ_DSN`) and use `app.DB.Reader()` and `app.DB.Writer()` to route queries. Disabled by default.
- **GORM integration**: Access a configured `*gorm.DB` via `app.GORM()` or run `app.AutoMigrate(&models...)` for schema management. GORM reuses the framework's existing database pool.
- **Query logging**: Enable `DATABASE_QUERY_LOGGING=true` to log every SQL statement with timing and error details through a transparent database/sql driver wrapper.

**Configuration Options:**

```properties
DATABASE_TYPE=postgres
DATABASE_HOST=127.0.0.1
DATABASE_PORT=5432
DATABASE_USER=postgres
DATABASE_PASS=postgres
DATABASE_NAME=myapp
DATABASE_SSL_MODE=disable

# Optional pool tuning
DATABASE_MAX_OPEN_CONNS=25
DATABASE_MAX_IDLE_CONNS=25
DATABASE_CONN_MAX_LIFETIME=15m

# Optional query logging (for development)
DATABASE_QUERY_LOGGING=true
```

**Usage Example in Your App:**

```go
// Run a health check
if err := app.DB.HealthCheck(r.Context()); err != nil {
    http.Error(w, "database unavailable", http.StatusServiceUnavailable)
    return
}

// Route reads to the read replica (or main pool when not configured)
rows, err := app.DB.Reader().QueryContext(r.Context(), "SELECT id, email FROM users")

// Route writes to the write pool
_, err = app.DB.Writer().ExecContext(r.Context(), "UPDATE users SET last_login = ? WHERE id = ?", now, id)

// Run code in a transaction
err := app.Transaction(r.Context(), func(tx *sql.Tx) error {
    _, err := tx.Exec("INSERT INTO users (email) VALUES (?)", email)
    return err
})

// Use GORM for ORM-style queries
gormDB, err := app.GORM()
if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
var users []User
gormDB.Find(&users)

// Run GORM AutoMigrate
_ = app.AutoMigrate(&User{}, &Post{})
```

### Examples

```sh
# Create migration with sql format
regius make migration create_users --format=sql

# Reverse last 2 migrations
regius migrate down 2

# Reverse all migrations
regius migrate down all

# Get help for any command
regius make migration --help
regius migrate --help
```

- **Rate Limiting Middleware**: Protect your application from abuse and DDoS attacks with flexible rate limiting.

  - Two algorithms: **Token Bucket** (steady request patterns) and **Sliding Window** (accurate for burst traffic)
  - Multiple storage backends: **In-memory** (fastest), **Redis** (distributed), and **Badger** (embedded distributed)
  - Configurable limits: Requests per time window (e.g., 100 requests per minute)
  - IP whitelisting: Exclude specific IPs or CIDR ranges (e.g. `10.0.0.0/8`, `::1/128`) from rate limiting — IPv4 and IPv6 supported
  - Proxy support: Trust X-Forwarded-For (first IP) and X-Real-IP headers
  - Standard HTTP headers: X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Window, Retry-After
  - Per-path rate limiting: Each route path gets its own rate limit
  - Apply globally or to specific routes (API, auth, etc.)

  **Usage Example in Your App:**

```go
  // In routes.go - Apply to all routes
  a.use(a.Middleware.RateLimit)

  // In routes-api.go - Apply to API routes
  r.Use(a.Middleware.APIRateLimit)

  // Or apply to specific routes
  r.Post("/login", a.Middleware.RateLimitStrict(a.Handlers.Login))
```

  **Configuration Options:**

```go
config := regius.RateLimiterConfig{
    Enabled:    true,                                       // Enable/disable rate limiting
    Algorithm:  regius.RateLimiterAlgorithmSlidingWindow,   // "token_bucket" or "sliding_window"
    Requests:   100,                                        // Maximum requests per window
    Window:     time.Minute,                                // Time duration (time.Second, time.Minute, time.Hour)
    Storage:    "",                                         // "" for in-memory, "redis" or "badger"
    TrustProxy: true,                                       // Trust proxy headers
    Whitelist:  []string{"127.0.0.1", "::1", "10.0.0.0/8"}, // IPs/CIDRs to exclude
}
```

  **Testing:**
  You can exercise the rate limiter with any HTTP load tool (e.g. `hey`, `wrk`, or a small `curl` loop) against a rate-limited route in your app.

  **Documentation:**

  - Full documentation: `RATE_LIMITER.md`
  - Implementation details: `RATE_LIMITER_IMPLEMENTATION.md`
  - Testing guide and quick start: see the `test-tools/` directory in a generated app

- **CORS Middleware**: Handle Cross-Origin Resource Sharing out of the box with flexible configuration.

  - Opt-out by default: CORS is enabled automatically with sensible defaults
  - Configurable origins: Allow specific domains or use wildcards
  - Configurable methods and headers: Control which HTTP methods and headers are permitted
  - Preflight support: Automatic handling of OPTIONS requests
  - Credentials support: Allow cookies and authorization headers in cross-origin requests
  - Apply globally or to specific route groups

  **Usage Example in Your App:**

```go
// CORS is applied globally by default when CORS_ENABLED=true (or unset)
// No additional code is required

// To apply CORS only to API routes, disable global CORS in .env:
// CORS_ENABLED=false
// Then manually apply in your routes file:
r.Group(func(mux chi.Router) {
  mux.Use(a.CORS(regius.CORSConfig{
      Enabled:        true,
      AllowedOrigins: []string{"https://app.example.com"},
      AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
      AllowCredentials: true,
  }))
  // API routes here
})
```

  **Configuration Options:**

```go
config := regius.CORSConfig{
    Enabled:            true,                                                // Enable/disable CORS
    AllowedOrigins:     []string{"*"},                                       // Allowed origins (use "*" for any)
    AllowedMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}, // Allowed HTTP methods
    AllowedHeaders:     []string{"Accept", "Authorization", "Content-Type"}, // Allowed request headers
    ExposedHeaders:     []string{},                                          // Headers exposed to the client
    MaxAge:             300,                                                 // Preflight cache duration in seconds
    AllowCredentials:   true,                                                // Allow cookies/auth headers
    OptionsPassthrough: false,                                               // Let OPTIONS requests pass through
    Debug:              false,                                               // Enable debug logging
}
```

- **Security Headers Middleware**: Set a bundle of HTTP security response headers out of the box — an Express "helmet" equivalent — to harden every response against XSS, clickjacking, MIME-sniffing, and SSL-downgrade attacks.

  - Opt-in by default: disabled unless `SECURITY_HEADERS_ENABLED=true`
  - Helmet-style safe defaults applied automatically: Content-Security-Policy (`default-src 'self'`), `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy`, Cross-Origin-Opener/Resource-Policy, and more
  - HSTS auto-gated: `Strict-Transport-Security` is only emitted when `SECURE=true`, so it never locks you out of local dev over `http://localhost`
  - Per-header overrides via environment variables (CSP, HSTS max-age, Referrer-Policy, etc.)
  - Non-blocking: headers are set before the downstream handler, so a route can still override any header via `w.Header().Set(...)`

  **Usage Example in Your App:**

```go
// Security headers are applied globally when SECURITY_HEADERS_ENABLED=true.
// No additional code is required.

// To override a header for a specific route, set it in the handler:
func (a *App) WidgetShow(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src https://cdn.example.com")
  // ...
}

// Or build the middleware manually for a route group:
r.Group(func(mux chi.Router) {
  mux.Use(a.SecurityHeaders(regius.SecurityHeadersConfig{
      Enabled:                true,
      ContentSecurityPolicy:  "default-src 'self'; script-src 'self'",
      HSTSIncludeSubDomains:  true,
  }))
  // routes here
})
```

  **Configuration Options:**

```go
config := regius.SecurityHeadersConfig{
    Enabled:                       true,                 // Master toggle
    ContentSecurityPolicy:         "default-src 'self'", // Empty -> default
    HSTSMaxAge:                    31536000,             // 0 -> 1 year default
    HSTSIncludeSubDomains:         true,                 // Add includeSubDomains
    HSTSPreload:                   false,                // Add preload
    ReferrerPolicy:                "strict-origin-when-cross-origin",
    XFrameOptions:                 "SAMEORIGIN",
    XPermittedCrossDomainPolicies: "none",
    CrossOriginOpenerPolicy:       "same-origin",
    CrossOriginResourcePolicy:     "same-origin",
    XDNSPrefetchControl:           "off",
}
```

- **API Key Authentication Middleware**: Authenticate API requests with API keys, complementing the existing session/cookie auth used by web routes.

  - Opt-in via `API_KEY_AUTH_ENABLED`; apply to API route groups only (not global) so cookie-authed web routes are unaffected
  - Multiple key sources (in order): `Authorization: Bearer <key>` (or configured scheme), `X-API-Key` header, and an opt-in query param (`API_KEY_QUERY_PARAM`, disabled by default since keys in URLs leak via logs/referrers)
  - Flexible validation backends (by precedence): a pluggable `Validator` func (e.g. DB-backed keys), a cache-backed `APIKeyStore` (with revocation), or a static list of `Keys` compared in constant time (`crypto/subtle`)
  - `CacheAPIKeyStore` adapts the framework cache (Redis/Badger); entries are keyed by the SHA-256 of the raw key, so raw keys are never persisted
  - On success, the `APIKeyIdentity` is stored in the request context — retrieve it with `regius.APIKeyFromContext(ctx)`
  - On failure: `401` with `WWW-Authenticate`, `Cache-Control: no-store`, and a JSON body. The raw key is never logged

  **Usage Example in Your App:**

```go
// Apply to your API route group (routes.go). API key auth is NOT global.
r.Group(func(mux chi.Router) {
  mux.Use(a.APIKeyAuth(regius.APIKeyAuthConfig{
      Enabled: true,
      Keys:    []string{"client-1-secret", "client-2-secret"},
  }))
  // API routes here...
})

// Or use env-driven config (set API_KEY_AUTH_ENABLED=true and API_KEYS in .env):
mux.Use(a.APIKeyAuth(a.APIKeyAuthCfg()))

// DB-backed keys via a custom validator:
mux.Use(a.APIKeyAuth(regius.APIKeyAuthConfig{
  Enabled: true,
  Validator: func(key string) (regius.APIKeyIdentity, bool) {
      // look up key in DB; return identity if valid
      return regius.APIKeyIdentity{ID: "user-42"}, true
  },
}))

// Retrieve the authenticated caller in a handler:
func (a *App) SomeAPIHandler(w http.ResponseWriter, r *http.Request) {
  id, ok := regius.APIKeyFromContext(r.Context())
  if !ok { /* unauthorized */ }
  // use id.ID, id.Metadata...
}
```

  **Configuration Options:**

```go
config := regius.APIKeyAuthConfig{
    Enabled:    true,               // Master toggle
    Keys:       []string{"secret"}, // Static valid keys (constant-time compare)
    Validator:  nil,                // Pluggable func(key) (identity, ok); takes precedence
    Store:      nil,                // APIKeyStore (e.g. CacheAPIKeyStore) for lookup/revocation
    Header:     "Authorization",    // Primary header (default "Authorization")
    Scheme:     "Bearer",           // Scheme prefix for Header (default "Bearer")
    AltHeader:  "X-API-Key",        // Secondary header, no prefix (default "X-API-Key")
    QueryParam: "",                 // Opt-in query param name (default "" = off)
    Realm:      "api",              // Used in WWW-Authenticate (default "api")
}

// Cache-backed store (keys hashed with SHA-256, never stored raw):
store := regius.NewCacheAPIKeyStore(a.Cache, "apikey:")
_ = store.Set("client-secret", regius.APIKeyIdentity{ID: "client-1"}, 0)
_ = store.Revoke("client-secret") // invalidate later
```

  **Environment Variables:**

```properties
API_KEY_AUTH_ENABLED=false
API_KEYS=                          # comma-separated list of valid keys
API_KEY_HEADER=Authorization
API_KEY_SCHEME=Bearer
API_KEY_ALT_HEADER=X-API-Key
API_KEY_QUERY_PARAM=               # empty = disabled
API_KEY_REALM=api
```

- **Request ID Tracing Middleware**: Stamp every request with a unique correlation ID for log correlation, distributed tracing, and client-side debugging.

  - Enabled by default: a request ID is generated for every request
  - Incoming ID reuse: reads an incoming ID from the request header (e.g. from a proxy/gateway) and reuses it verbatim, so a single user action can be correlated across services
  - Echoed on the response: the ID is written to a response header so clients can map a response back to server logs / support tickets
  - Configurable ID format: `uuid` (default), `xid` (sortable/short), `short` (12-char base62), or `default` (chi-style `host/random-counter`)
  - Pluggable generator: supply a custom `Generator` func for custom schemes (e.g. ULID, tenant-prefixed IDs)
  - Context propagation: the ID is stored in the request context under chi's `RequestIDKey`, so chi's `middleware.GetReqID` and request logger keep working — retrieve it in a handler with `regius.RequestIDFromContext(ctx)`
  - Hardening: incoming IDs are trimmed and capped (128 chars) to prevent log injection / header abuse

  **Usage Example in Your App:**

```go
// Request ID tracing is applied globally by default.
// No additional code is required.

// Retrieve the request ID in a handler:
func (a *App) SomeHandler(w http.ResponseWriter, r *http.Request) {
  id, ok := regius.RequestIDFromContext(r.Context())
  if ok {
      a.InfoLog.Printf("handling request %s", id)
  }
  // ...
}

// Or build the middleware manually for a route group:
r.Group(func(mux chi.Router) {
  mux.Use(a.RequestID(regius.RequestIDConfig{
      Enabled:        true,
      Format:         regius.RequestIDFormatXID,
      ResponseHeader: "X-Correlation-ID",
  }))
  // routes here
})
```

  **Configuration Options:**

```go
config := regius.RequestIDConfig{
    Enabled:        true,                       // Master toggle (default true)
    Header:         "X-Request-ID",             // Request header to read incoming ID from
    ResponseHeader: "X-Request-ID",             // Response header to echo the ID on ("" = don't echo)
    Format:         regius.RequestIDFormatUUID, // "uuid" | "xid" | "short" | "default"
    Generator:      nil,                        // Optional override of Format
}
```

  **Environment Variables:**

```properties
REQUEST_ID_ENABLED=true
REQUEST_ID_HEADER=X-Request-ID
REQUEST_ID_RESPONSE_HEADER=X-Request-ID
REQUEST_ID_FORMAT=uuid                   # uuid | xid | short | default
```

- **Request Sanitization Middleware**: Neutralize XSS at the request boundary by sanitizing query params, form-encoded values, and selected request headers with [bluemonday](https://github.com/microcosm-cc/bluemonday) before downstream handlers ever see them.

  - Defense-in-depth: enabled by default in scaffolded apps (`REQUEST_SANITIZATION_ENABLED=true`)
  - Two policies via env: **strict** (default — strips all HTML, returns safe text) and **ugc** (allows a safe HTML subset like `<b>`, `<a>` for comment-style fields)
  - Sanitizes URL query params, form-encoded POST values (including multipart text fields), and a configurable allowlist of request headers (default `Referer`, `User-Agent`)
  - JSON-safe: `application/json` request bodies are **never** parsed or consumed, so API handlers keep full access to `r.Body`
  - Path exemption: routes matching `REQUEST_SANITIZATION_EXEMPT` (default `/api/.*`) bypass sanitization entirely, mirroring the CSRF (`NoSurf`) exemption
  - Standalone helpers for targeted use: `r.Sanitize(s)` / `r.Sanitizer()` (app-configured policy) and package-level `regius.Sanitize(s)` (strict)
  - Non-destructive: clean values are left byte-for-byte intact; only values containing HTML are rewritten
  - Don't sanitize structural headers (`Authorization`, `Cookie`, `X-CSRF-Token`, `Content-*`, `X-Forwarded-*`, `X-Request-ID`) — doing so breaks routing, auth, and tracing. The default allowlist avoids these.

  **Usage Example in Your App:**

```go
// Request sanitization is applied globally when REQUEST_SANITIZATION_ENABLED=true.
// No additional code is required.

// Targeted sanitization in a handler (e.g. before storing user input):
func (a *App) StoreComment(w http.ResponseWriter, r *http.Request) {
  raw := r.FormValue("comment")
  safe := a.Sanitize(raw) // uses the app's configured policy
  // store safe...
}

// Or build the middleware manually for a route group:
r.Group(func(mux chi.Router) {
  mux.Use(a.RequestSanitizer(regius.RequestSanitizerConfig{
      Enabled: true,
      Policy:  regius.SanitizePolicyUGC, // allow safe HTML subset
      Headers: []string{"Referer"},
  }))
  // routes here...
})
```

  **Configuration Options:**

```go
config := regius.RequestSanitizerConfig{
	Enabled: true,                              // Master toggle
	Policy:  regius.SanitizePolicyStrict,       // "strict" (default) | "ugc"
	Query:   regius.BoolPtr(true),              // Sanitize URL query params (default true)
	Form:    regius.BoolPtr(true),              // Sanitize form-encoded values (default true)
	Headers: []string{"Referer", "User-Agent"}, // Header allowlist (default none)
	Exempt:  "/api/.*",                         // Regex of paths to skip (default "/api/.*")
	Custom:  nil,                               // Optional *bluemonday.Policy override
}

// BoolPtr is a tiny helper to set *bool fields (nil defaults to true):
regius.BoolPtr(false) // explicitly disable a scope
```

  **Environment Variables:**

```properties
REQUEST_SANITIZATION_ENABLED=true
REQUEST_SANITIZATION_POLICY=strict         # strict | ugc
REQUEST_SANITIZATION_QUERY=true
REQUEST_SANITIZATION_FORM=true
REQUEST_SANITIZATION_HEADERS=Referer,User-Agent
REQUEST_SANITIZATION_EXEMPT=/api/.*
```

- **IP Whitelist/Blacklist Middleware**: Allow or deny requests based on the client IP, using static lists and optional runtime (cache/DB-backed) decisions for fail2ban-style blocking.

  - Opt-in via `IP_FILTER_ENABLED`; applied globally (after RealIP) so denied requests short-circuit before heavier middleware runs
  - Allow/deny lists of IPs **or CIDR ranges** (e.g. `10.0.0.0/8`, `192.168.1.5`, `::1/128`); bare IPs are treated as `/32` (IPv4) or `/128` (IPv6). IPv4 and IPv6 both supported
  - **Deny-wins** semantics: a matching `Deny` entry always blocks; when `Allow` is non-empty, any IP not in `Allow` is blocked. With neither list set, all IPs are allowed
  - Invalid entries are logged and skipped (non-fatal) — one bad CIDR won't take down the filter
  - `TrustProxy` (default off) reads the client IP from `X-Forwarded-For` (first entry) or `X-Real-IP` instead of `RemoteAddr`. Only enable behind a trusted reverse proxy, otherwise the header can be spoofed to bypass the filter
  - Optional pluggable `IPChecker` interface for dynamic, DB/cache-backed decisions: `DecisionAllow`/`DecisionDeny` override the static lists, `DecisionNone` defers to them. Checker errors fail open (the static baseline still applies)
  - `CacheIPChecker` adapts the framework cache (Redis/Badger) for runtime block/unblock without restart; entries namespaced under `ipfilter:`
  - Blocked requests respond with a configurable status (default 403) + a JSON body and `Cache-Control: no-store`

  **Usage Example in Your App:**

```go
// IP filtering is applied globally when IP_FILTER_ENABLED=true.
// No additional code is required.

// Or build the middleware manually for a route group (e.g. restrict admin):
r.Group(func(mux chi.Router) {
	mux.Use(a.IPFilter(regius.IPFilterConfig{
		Enabled: true,
		Allow:   []string{"10.0.0.0/8", "192.168.1.0/24"},
		Deny:    []string{"10.0.0.99"},
	}))
	// admin routes here...
})

// Runtime (fail2ban-style) blocking via a cache-backed checker:
checker := regius.NewCacheIPChecker(a.Cache, "ipfilter:")
_ = checker.Block("203.0.113.50", 3600) // block for 1 hour
_ = checker.Unblock("203.0.113.50")     // unblock later

mux.Use(a.IPFilter(regius.IPFilterConfig{
	Enabled: true,
	Deny:    []string{"198.51.100.0/24"}, // static baseline
	Checker: checker,                     // dynamic layer on top
}))
```

  **Configuration Options:**

```go
config := regius.IPFilterConfig{
	Enabled:    true,                     // Master toggle
	Allow:      []string{"10.0.0.0/8"},   // Only these networks pass (deny-wins)
	Deny:       []string{"10.0.0.99"},    // Always blocked
	TrustProxy: false,                    // Read X-Forwarded-For/X-Real-IP (default false)
	StatusCode: 403,                      // Block response status (default 403)
	Message:    "ip address not allowed", // Block response message
	Checker:    nil,                      // Optional IPChecker (e.g. CacheIPChecker)
}

// Cache-backed checker for runtime decisions (TTL in seconds; 0 = no expiry):
checker := regius.NewCacheIPChecker(a.Cache, "ipfilter:")
_ = checker.Block(ip, 3600) // DecisionDeny
_ = checker.Allow(ip, 0)    // DecisionAllow
_ = checker.Unblock(ip)     // remove decision -> defer to static lists
```

  **Environment Variables:**

```properties
IP_FILTER_ENABLED=false
IP_FILTER_ALLOW=                          # comma-separated IPs/CIDRs to permit
IP_FILTER_DENY=                           # comma-separated IPs/CIDRs to block (deny-wins)
IP_FILTER_TRUST_PROXY=false               # read X-Forwarded-For/X-Real-IP
IP_FILTER_STATUS_CODE=403
IP_FILTER_MESSAGE=
```

- **Internationalization (i18n)**: Ship multi-language apps out of the box with locale detection, translation file management, and a built-in language selector.

  - Enabled by default (`I18N_ENABLED=true`); applied globally in `routes.go`
  - Locale resolution order: `LOCALE_COOKIE_NAME` cookie → `Accept-Language` header → `DEFAULT_LOCALE`
  - Default supported locales are **English** (`en`) and **Spanish** (`es`), configurable via `SUPPORTED_LOCALES`
  - Generated apps embed translations under `locales/<code>/<code>.yaml` and load them in `init.regius.go`
  - Add new locales with `./regius make locale <code>` (e.g. `./regius make locale fr`)

  **Usage in templ views:**

```go
import "github.com/hbarral/regius/i18n"

templ Hello(name string) {
<p>{ i18n.T(ctx, "navbar.welcome", i18n.M{"name": name}) }</p>
}
```

  **Usage in Jet/Go templates:**

```html
<p>{{T "navbar.home"}}</p>
<html lang="{{.Locale}}">
```

  **Environment Variables:**

```properties
I18N_ENABLED=true
DEFAULT_LOCALE=en
SUPPORTED_LOCALES=en,es
LOCALE_COOKIE_NAME=locale
```

- **Server-Sent Events (SSE)**: Push real-time updates to browsers over standard HTTP with a built-in, zero-dependency SSE broker.

  - Global broker available on every app: `app.SSE`
  - Broadcast to all connected clients or send to a specific client
  - JSON helper: `app.SSEBroadcastJSON(event, payload)` marshals a payload and broadcasts it
  - Automatic client disconnect detection via request context cancellation
  - Works through the existing middleware stack (`RequestID`, `CORS`, `SecurityHeaders`, `SessionLoad`, etc.)
  - Generated apps include a visual SSE demo on the home page (`/sse/stream` and `/sse/ping`)

  **Usage Example in Your App:**

```go
// Broadcast a JSON event to every connected browser
_ = app.SSEBroadcastJSON("notification", map[string]string{
	"message": "Hello, world!",
})

// Or build an event manually and broadcast it
app.SSE.Broadcast(regius.SSEEvent{
	Event: "notification",
	Data:  []byte(`{"message":"Hello, world!"}`),
})

// Stream events from a handler
r.Get("/sse/stream", app.SSE.Handler())
```

  **Configuration Options:**

```go
config := regius.SSEEvent{
    ID:    "1",                   // Optional event id for the Last-Event-ID header
    Event: "update",              // Event name listeners can subscribe to
    Data:  []byte(`{"ok":true}`), // Raw event payload
    Retry: 3000,                  // Optional reconnection time in milliseconds
}
```

  **Environment Variables:**

```properties
# Disable the demo heartbeat in generated apps (enabled by default)
SSE_DEMO_HEARTBEAT=false
```

## 🚀 Getting Started

### Homebrew (macOS & Linux)

The easiest way to install Regius on macOS or Linux is with [Homebrew](https://brew.sh):

```sh
brew install hbarral/tap/regius
```

Verify the installation:

```sh
regius help
```

To upgrade to the latest release later on:

```sh
brew upgrade regius
```

### Download Binaries

Download the suitable binary for your operating system from the links below:

- [Linux](https://github.com/hbarral/regius/releases/download/v1.9.2/regius_Linux_x86_64.tar.gz)
- [Windows](https://github.com/hbarral/regius/releases/download/v1.9.2/regius_Windows_x86_64.zip)
- [Mac](https://github.com/hbarral/regius/releases/download/v1.9.2/regius_Darwin_x86_64.tar.gz)

<details>
  <summary>Build from Source</summary>

1. Clone the repository:

```sh
git clone https://github.com/hbarral/regius.git
cd regius
```

2. Build the project for your operating system:

```sh
go build -o regius ./cmd/cli
```

3. Run the binary:

```sh
./regius help
```

</details>

### Environment Variables

Upon creating a new application, `regius` generates a `.env` file with default configurations. You only need to fill in the required values. Below is an example of a complete `.env` file:

```plaintext
# Application name, without spaces
APP_NAME=testapp
APP_URL="http://localhost:4000"

# False for production, true for development
DEBUG=true

# The port should we listen on
PORT=4000
RPC_PORT=4001

# Server name, e.g, www.example.com
SERVER_NAME=localhost

# use https?
SECURE=false

# database configuration (sqlite is the default; a local file at data/<name>.db)
DATABASE_TYPE=sqlite
DATABASE_HOST=
# ...
```

<details>
  <summary>See the full .env example</summary>

```properties
# Application name, without spaces
APP_NAME=testapp
APP_URL="http://localhost:4000"

# False for production, true for development
DEBUG=true

# The port should we listen on
PORT=4000
RPC_PORT=4001

# Server name, e.g, www.example.com
SERVER_NAME=localhost

# use https?
SECURE=false

# database configuration (sqlite is the default; a local file at data/<name>.db)
DATABASE_TYPE=sqlite
DATABASE_HOST=
DATABASE_PORT=
DATABASE_USER=
DATABASE_PASS=
DATABASE_NAME=regius
DATABASE_SSL_MODE=

# minio settings
MINIO_ENDPOINT=
MINIO_KEY=
MINIO_SECRET=
MINIO_USESSL=
MINIO_REGION=
MINIO_BUCKET=

# sftp settings
SFTP_HOST=
SFTP_PORT=
SFTP_USER=
SFTP_PASS=

# webdav settings
WEBDAV_HOST=
WEBDAV_PORT=
WEBDAV_USER=
WEBDAV_PASS=
WEBDAV_USESSL=

# s3 settings
S3_KEY=
S3_SECRET=
S3_REGION=
S3_BUCKET=
S3_ENDPOINT=

# redis settings
REDIS_HOST=
REDIS_PASSWORD=
REDIS_PREFIX=testapp

# cache config redis or badger
CACHE=

# cookie settings
COOKIE_NAME=testapp
COOKIE_LIFETIME=1440
COOKIE_PERSISTS=true
COOKIE_SECURE=false
COOKIE_DOMAIN=localhost

# session store: cookie, mysql, mariadb, postgres or redis
SESSION_TYPE=cookie

# mail settings (SMTP_ENCRYPTION=tls | ssl | none)
SMTP_HOST=
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_PORT=
SMTP_ENCRYPTION=
MAIL_DOMAIN=
FROM_NAME=
FROM_ADDRESS=

# mail settings for api services
MAILER_API=
MAILER_KEY=
MAILER_URL=

# Template engine (used by CLI scaffolding only; at runtime each handler
# picks its engine via render.Jet(), render.Go(), or a templ component)
RENDERER=templ

# encryption key (32 characters long)
KEY=DPFtfVnxbtnXXRzVnRzrLxDzXXRh+Xft

# password hashing (algorithm: bcrypt | scrypt | argon2)
HASH_ALGORITHM=bcrypt
# bcrypt cost (4-31, default 12)
HASH_COST=12
# scrypt parameters
HASH_SCRYPT_N=32768
HASH_SCRYPT_R=8
HASH_SCRYPT_P=1
# argon2id parameters
HASH_ARGON2_MEMORY=65536
HASH_ARGON2_ITERATIONS=3
HASH_ARGON2_PARALLELISM=2

# types of files allowed to upload
ALLOWED_FILETYPES="image/png,image/jpeg,image/gif,application/pdf"
# 5MB
MAX_FILESIZE=5242880

# CORS configuration (enabled by default)
CORS_ENABLED=true
CORS_ALLOWED_ORIGINS="*"
CORS_ALLOWED_METHODS="GET,POST,PUT,DELETE,OPTIONS,PATCH,HEAD"
CORS_ALLOWED_HEADERS="Accept,Authorization,Content-Type,X-CSRF-Token"
CORS_EXPOSED_HEADERS=""
CORS_ALLOW_CREDENTIALS=true
CORS_MAX_AGE=300

# security headers (helmet equivalent, disabled by default).
# HSTS is only emitted when SECURE=true.
SECURITY_HEADERS_ENABLED=false
CONTENT_SECURITY_POLICY=default-src 'self'; script-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; style-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; img-src 'self' data:; font-src 'self' https://cdn.jsdelivr.net; frame-ancestors 'self'
HSTS_MAX_AGE=31536000
HSTS_INCLUDE_SUBDOMAINS=true
HSTS_PRELOAD=false
REFERRER_POLICY=strict-origin-when-cross-origin
X_FRAME_OPTIONS=SAMEORIGIN

# request id tracing (enabled by default)
REQUEST_ID_ENABLED=true
REQUEST_ID_HEADER=X-Request-ID
REQUEST_ID_RESPONSE_HEADER=X-Request-ID
REQUEST_ID_FORMAT=uuid

# request sanitization for XSS prevention (on by default)
REQUEST_SANITIZATION_ENABLED=true
REQUEST_SANITIZATION_POLICY=strict
REQUEST_SANITIZATION_QUERY=true
REQUEST_SANITIZATION_FORM=true
REQUEST_SANITIZATION_HEADERS=Referer,User-Agent
REQUEST_SANITIZATION_EXEMPT=/api/.*

# ip whitelist/blacklist (opt-in). Deny always wins over allow.
IP_FILTER_ENABLED=false
IP_FILTER_ALLOW=
IP_FILTER_DENY=
IP_FILTER_TRUST_PROXY=false
IP_FILTER_STATUS_CODE=403
IP_FILTER_MESSAGE=

# github oauth
GITHUB_KEY=
GITHUB_SECRET=
GITHUB_CALLBACK=

# google oauth
GOOGLE_KEY=
GOOGLE_SECRET=
GOOGLE_CALLBACK=

# docker compose
POSTGRES_DB=
POSTGRES_USER=
POSTGRES_PASSWORD=

MYSQL_DATABASE=
MYSQL_USER=
MYSQL_PASSWORD=
MYSQL_ROOT_PASSWORD=
```

</details>

### Database Configuration

After creating a new application, a `.env` file is generated with the following database variables:

```properties
# database configuration
# supported types: postgres, postgresql, mysql, mariadb, sqlite, sqlite3
DATABASE_TYPE=postgres
DATABASE_HOST=127.0.0.1
DATABASE_PORT=5432
DATABASE_USER=postgres
DATABASE_PASS=postgres
DATABASE_NAME=myapp
DATABASE_SSL_MODE=disable

# Optional pool tuning
DATABASE_MAX_OPEN_CONNS=25
DATABASE_MAX_IDLE_CONNS=25
DATABASE_CONN_MAX_LIFETIME=15m

# Optional query logging
DATABASE_QUERY_LOGGING=true
```

Fill in these values with your database connection details. Migrations, seeds, and health checks use these environment variables directly — no additional configuration file required.

### Password Hashing

Regius provides a centralized password hashing utility accessible via `App.Hash`, supporting `bcrypt` (default), `scrypt`, and `argon2id`. The algorithm and its parameters are configured through environment variables:

```properties
HASH_ALGORITHM=bcrypt
HASH_COST=12
```

Use it anywhere you have access to the `*Regius` application instance:

```go
// Hash a password before storing it
hashed, err := h.App.Hash.Generate(plainPassword)

// Verify a password against a stored hash
ok, err := h.App.Hash.Compare(storedHash, plainPassword)
```

The `make auth` scaffolding uses `App.Hash` directly, so the generated handlers and user model stay hash-agnostic. Defaults preserve the previous behavior (bcrypt at cost 12), so existing password hashes continue to verify.

## 🎯 Usage

Each command has different options and parameters. Here are some basic usage examples:

- **Create a new application:**

```sh
./regius new myapp
```

  The scaffold defaults to the **templ** renderer (`github.com/a-h/templ` +
  [templui](https://github.com/templui/templui) + Tailwind v4) and the
  **sqlite** database, and ships a runnable auth scaffold (navbar, sign
  in/up/forgot/reset-password). After `regius new`, run the auth migration
  (`./regius migrate`) and `./regius migrate` then `go run .`:

```sh
cd myapp
./regius migrate        # creates users/tokens/remember_tokens on sqlite
go run .                # http://localhost:4000  (/ , /auth/signin)
```

  Optional flags:

  - `--db <type>`: pre-fill `DATABASE_TYPE` in the generated `.env`
    (`postgres`|`postgresql`|`mysql`|`mariadb`|`sqlite`|`sqlite3`).
    Defaults to `sqlite` (a local file at `data/<name>.db`, no server needed).
  - `--renderer <engine>`: template engine to scaffold — `templ` (default) |
    `jet` (`github.com/CloudyKit/jet/v6`, `*.jet` views with shared
    `views/layouts/*.jet` layouts, Tailwind v4 + Alpine.js for the same modern
    UI as the templ skeleton) |
    `go` (built-in `html/template`, `*.page.template` views with a shared
    `views/layouts/*.layout.template` layout system, Tailwind v4 + Alpine.js
    for the same modern UI as the templ skeleton). `regius make auth` and
    `regius make handler` also accept `--renderer`, falling back to the
    `RENDERER` env var then `templ`.
  - `-v`, `--verbose`: stream `go get` / `go mod tidy` output live instead of
    capturing it (the captured output is shown only on failure by default).

```sh
./regius new myapp --db postgres -v
./regius new jetapp --renderer jet
./regius new goapp --renderer go
```

- Show help commands:

```sh
./regius help
```

- Run a migration:

```sh
./regius migration
```

- Create a migration:

```sh
./regius make migration create_users_table
```

- Create a seed file and run it:

```sh
./regius make seed default_users
./regius db:seed
```

- Check current migration version:

```sh
./regius migrate version
```

- Create a model:

```sh
./regius make model User
```

- Create a GORM model:

```sh
./regius make gorm-model User
```

- Put the server in maintenance mode:

```sh
./regius down
```

- Bring the server back from maintenance mode:

```sh
./regius up
```

For more details about usage and commands, refer to the CLI help:

```sh
./regius help
```

## 🎨 Rendering Templates

Regius provides a unified `render.Template` interface for all three template engines: **jet**, **go**, and **templ**. The scaffolded app defaults to **templ** (`regius new --renderer templ`; switch with `--renderer jet|go`). Every handler calls the same `Page()` method — the only difference is how the `Template` is created.

> **templ build step:** templ views (`*.templ`) are compiled to Go (`*_templ.go`) by `templ generate`. The scaffolded `Makefile` runs `templ generate` as part of `build`, and `regius new`/`regius make auth`/`regius make handler` invoke it automatically for templ apps.

### Jet

```go
h.App.Render.Page(w, r, h.App.Render.Jet("home", nil), nil)
```

### Go

```go
// Single-file template (no layout)
h.App.Render.Page(w, r, h.App.Render.Go("home"), nil)

// Template inside a shared layout (page defines a "content" block)
h.App.Render.Page(w, r, h.App.Render.GoLayout("home", "base"), nil)
```

The `go` renderer uses `html/template`. Pages rendered with `GoLayout(name,
layout)` must define a `{{define "content"}}...{{end}}` block; the layout in
`views/layouts/<layout>.layout.template` executes it with
`{{template "content" .}}`. Component partials in `views/components/*.page.template`
are automatically available to every Go template.

### Templ

Templ components implement `render.Template` natively — pass them directly with no wrapper and no registration:

```go
h.App.Render.Page(w, r, views.Home(), &render.TemplateData{Data: data})
```

### Mixing Engines

Each handler independently chooses its engine, so you can mix jet, go, and templ in the same application without any global `RENDERER` setting.

## 🎨 Tailwind CSS

The `templ`, `go`, and `jet` renderers ship a pre-built stylesheet at
`public/css/output.css` so generated apps look correct immediately. The source
of truth is `assets/css/input.css` plus the Tailwind utility classes used in
your views.

To customize styles you need the **Tailwind CSS CLI** installed:

```sh
# Rebuild once
make tailwind

# Or watch for changes (requires go-task)
go-task tailwind
```

Scanning by renderer:

- `go`: `*.page.template`, `*.layout.template`, `*.js`
- `jet`: `*.jet`, `*.js`
- `templ`: `*.templ`, `*.js`, and the `templui` component library

## 🤝 Contributing

Contributions are welcome! Please follow the GitHub flow for contributions:

1. Fork the project.
2. Create a new branch (`git checkout -b feature-new-feature`).
3. Make your changes and commit (`git commit -am 'Add new feature'`).
4. Push to the branch (`git push origin feature-new-feature`).
5. Open a Pull Request.

## 📄 License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.

---

_Made with 🩵 by Héctor Barral._
