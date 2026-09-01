# AGENTS.md - Regius Development Guidelines

This file contains guidelines and commands for agentic coding assistants working on the Regius codebase.

## Project Overview

Regius is a CLI application for building web pages in Go, inspired by Laravel. It provides tools for database migrations, code generation, and web application scaffolding.

This repository uses a **split-module layout** to keep the CLI binary small:

- **Root module** (`github.com/hbarral/regius`): the web framework — `regius.go`, `render/`, `session/`, `mailer/`, `cache/`, `filesystems/`, etc. This is what end-user apps import.
- **CLI module** (`github.com/hbarral/regius/cli`): the command-line tool — a separate Go module under `cli/` with its own `go.mod` and a minimal dependency set. It does **not** import the root module; instead it has a lightweight `Backend` struct (`cli/backend.go`) that reimplements the few methods the CLI needs (RandomString, CreateMigration, DSN building, golang-migrate wrappers, Seeder). This keeps the CLI binary at ~12 MB instead of ~35 MB.
- **Entry point** (`cmd/cli/main.go`): a thin `package main` that imports `github.com/hbarral/regius/cli` and calls `cli.Execute()`. It lives in the root module so `go build ./cmd/cli` works from the repo root.

The starter app template is embedded directly in the CLI module as `cli/_skeleton/` (embedded via `//go:embed all:_skeleton` in `cli/copy-files.go`), so `regius new <name>` writes the skeleton from the embedded filesystem.
The underscore prefix makes Go's build tool ignore the skeleton directory, while `all:` lets the embed include it. The skeleton source uses the internal module name `regius-app` for its imports; `regius new` rewrites the literal `regius-app` to the chosen app name via `updateSource()` in `cli/helpers.go`.

Code-generation templates are embedded in `cli/templates/` (via `//go:embed templates`).

When changing the skeleton (`cli/_skeleton/`), verify end-to-end by building the CLI and running `regius new smoketest && (cd smoketest && go build ./...)`. `regius new` runs `templ generate` for the default `--renderer templ`, so the generated app builds directly.
The skeleton's own `go.mod`/`go.sum` are intentionally absent — it is not a standalone module; its correctness is validated by generating an app from it and building that app. When changing renderer-aware scaffolding, also smoke the matrix: `regius new jetapp --renderer jet`, `regius new goapp --renderer go`, and `regius make auth`/`regius make handler --renderer <e>` in each, then `go build ./...`.

> **Release coupling:** the embedded skeleton's handlers use the head `Render.Page(view render.Template, data)` API. Before end-user apps build out-of-the-box (without a local `replace`), a `github.com/hbarral/regius` release carrying that API must be published and `cli/templates/go_mod` bumped to require it; until then verify generated apps with a `replace github.com/hbarral/regius => ../regius` directive in the app's `go.mod`.

## Build/Lint/Test Commands

### Testing
```bash
# Run all tests (both root and CLI modules)
make test
# or
Go test -v ./...
go test -v ./cli/...

# Run tests with coverage
make coverage
# or
go test -cover ./...

# Run tests and view coverage in browser
make cover
# or
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Run a specific test file
go test -v ./render/render_test.go

# Run a specific test function
go test -v -run TestRender_Page ./render/

# Run tests in a specific package
go test -v ./session/
```

### Building
```bash
# Build CLI for current platform
make build

# Build to a specific path
go build -ldflags "-s -w" -o ./dist/regius ./cmd/cli

# Cross-platform builds (used in CI)
GOOS=linux GOARCH=amd64 go build -o bin/regius-linux ./cmd/cli
GOOS=windows GOARCH=amd64 go build -o bin/regius-windows.exe ./cmd/cli
GOOS=darwin GOARCH=amd64 go build -o bin/regius-mac ./cmd/cli

# Build the CLI module directly (for testing CLI deps in isolation)
cd cli && go build -o /tmp/regius-cli .
```

### Formatting & Linting
```bash
# Format code (no linting config found, use standard Go tools)
go fmt ./...

# Vet for suspicious code
go vet ./...

# Run golangci-lint if available (recommended to add)
golangci-lint run
```

## Code Style Guidelines

### Go Conventions

#### Naming Conventions
- **Packages**: lowercase, single word (e.g., `render`, `session`, `mailer`)
- **Exported functions/types**: PascalCase (e.g., `RenderPage`, `InitSession`)
- **Unexported functions/types**: camelCase (e.g., `createDirIfNotExist`)
- **Variables**: camelCase (e.g., `rootPath`, `cookieLifetime`)
- **Constants**: PascalCase for exported, camelCase for unexported
- **Test functions**: `TestXxx` format (e.g., `TestRender_Page`)

#### File Organization
- Main package files in root directory
- Subpackages in subdirectories (e.g., `render/`, `session/`, `mailer/`)
- Test files named `*_test.go` in same directory as code
- CLI commands in `cmd/cli/` directory

#### Imports
```go
import (
    "standard/library"
    "third/party/package"

    "github.com/hbarral/regius/internal/package"
)
```
- Standard library imports first
- Third-party imports second
- Local imports last (with blank line separation)
- Group related imports together

### Code Structure Patterns

#### Error Handling
```go
// Always check and handle errors
if err != nil {
    return err
}

// Or wrap with context
if err != nil {
    return fmt.Errorf("failed to create directory: %w", err)
}

// For validation errors, use the Validation struct
func (v *Validation) Check(ok bool, key, message string) {
    if !ok {
        v.AddError(key, message)
    }
}
```

#### Function Signatures
```go
// Methods on structs use pointer receivers
func (r *Regius) CreateDirIfNotExist(path string) error

// Factory functions return pointers
func (r *Regius) Validator(data url.Values) *Validation

// Simple functions use value receivers where appropriate
func (v *Validation) Valid() bool
```

#### Type Definitions
```go
// Structs with proper JSON tags where needed
type Database struct {
    DataType string `json:"data_type"`
    Pool     *sql.DB `json:"-"`
}

// Configuration structs
type cookieConfig struct {
    name     string
    lifetime string
    persist  string
    secure   string
    domain   string
}
```

### Testing Patterns

#### Table-Driven Tests
```go
var pageData = []struct {
    name          string
    renderer      string
    template      string
    errorExpected bool
    errorMessage  string
}{
    {"go_page", "go", "home", false, "Error rendering Go template"},
    {"jet_page", "jet", "home", false, "Error rendering Jet template"},
}

func TestRender_Page(t *testing.T) {
    for _, e := range pageData {
        // test implementation
    }
}
```

#### Setup/Teardown
```go
func TestMain(m *testing.M) {
    // Setup code
    setupTestEnvironment()

    code := m.Run()

    // Teardown code
    cleanupTestEnvironment()

    os.Exit(code)
}
```

### Database & Migration Patterns

#### Migration Commands
```bash
# Run migrations up
./regius migrate
# or
./regius migrate up

# Run migrations down
./regius migrate down

# Reset all migrations
./regius migrate reset
```

#### Database Types
- PostgreSQL
- MySQL
- SQLite (implied)

### Session Management

#### Session Types
- Cookie-based sessions
- Redis-backed sessions
- Database-backed sessions (PostgreSQL/MySQL)

### File System Abstractions

#### Supported Filesystems
- Local filesystem
- S3
- MinIO
- SFTP
- WebDAV

### Email/Mailer System

#### Template Support
- Plain text templates
- HTML templates
- Uses `github.com/vanng822/go-premailer` for HTML processing

### Web Framework Components

#### Router
- Uses Chi router (`github.com/go-chi/chi/v5`)

#### Template Engines
The scaffolded app defaults to **templ** (`github.com/a-h/templ`) with the [templui](https://github.com/templui/templui) component library + Tailwind v4; select another engine at create time with `regius new <name> --renderer jet|go`.
- templ (`github.com/a-h/templ`) — default; ships a full templui/Tailwind UI (navbar, auth screens, theme switcher) and a `templ generate` build step. Views live in `views/*.templ` (+ generated `*_templ.go`); templ components satisfy `render.Template` natively, so handlers pass them straight to `Render.Page`.
- Jet templates (`github.com/CloudyKit/jet/v6`) — same modern shadcn-style UI as the templ skeleton, implemented with `*.jet` views, shared `views/layouts/*.jet` layouts, Tailwind v4, and Alpine.js. Handlers call `Render.Jet("name", vars)`.
- Go templates (built-in) — same modern shadcn-style UI as the templ skeleton, implemented with `html/template`, Tailwind v4, and Alpine.js. Views live in `views/*.page.template` and share layouts in `views/layouts/*.layout.template`; component partials live in `views/components/*.page.template`. Handlers call `Render.Go("name")` for single-file pages or `Render.GoLayout("name", "layout")` for layout-wrapped pages.

#### Middleware
- CSRF protection (`github.com/justinas/nosurf`)
- Session management
- Custom middleware for maintenance mode
- CORS middleware (`r.CORS`) — opt-out by default via `CORS_ENABLED`
- Rate limiting middleware (`r.RateLimiter`) — token bucket / sliding window, in-memory/Redis/Badger; CIDR-aware whitelist (IPv4/IPv6, parsed once at construction); client IP extracted via the shared `clientIPAddress` helper (`net.SplitHostPort` for correct IPv6, first IP of `X-Forwarded-For` when `TrustProxy`)
- Security headers middleware (`r.SecurityHeaders`) — helmet equivalent; opt-in via `SECURITY_HEADERS_ENABLED` (enabled in scaffolded apps)
- API key auth middleware (`r.APIKeyAuth`) — static keys (constant-time) / pluggable `Validator` / cache `Store`; opt-in via `API_KEY_AUTH_ENABLED`; apply to API route groups (not global)
- Request ID tracing middleware (`r.RequestID`) — stamps every request with a correlation ID; enabled by default (opt-out via `REQUEST_ID_ENABLED`); reuses an incoming ID from `REQUEST_ID_HEADER` (cross-service correlation), otherwise generates one (`REQUEST_ID_FORMAT`: `uuid` default | `xid` | `short` | `default`); echoes on `REQUEST_ID_RESPONSE_HEADER`; stored in context under chi's `RequestIDKey` (interoperable with `middleware.GetReqID`); retrieve via `regius.RequestIDFromContext(ctx)`; wired globally in `routes.go`
- Request sanitization middleware (`r.RequestSanitizer`) — XSS prevention via [bluemonday](https://github.com/microcosm-cc/bluemonday); sanitizes query params, form values, and a header allowlist; opt-in via `REQUEST_SANITIZATION_ENABLED` (enabled in scaffolded apps); JSON bodies and `/api/*` exempt by default; wired globally in `routes.go`
- IP whitelist/blacklist middleware (`r.IPFilter`) — allow/deny lists of IPs & CIDR ranges (IPv4/IPv6); deny-wins; pluggable `IPChecker` (e.g. `CacheIPChecker` for runtime fail2ban-style blocking); opt-in via `IP_FILTER_ENABLED`; wired globally in `routes.go` (after `RealIP`)
- Internationalization middleware (`r.Language`) — detects locale from the `LOCALE_COOKIE_NAME` cookie, then the `Accept-Language` header, then `DEFAULT_LOCALE`; enabled by default (`I18N_ENABLED=true`); wired globally in `routes.go`; the resolved locale is available via `i18n.Locale(ctx)` and `i18n.T(ctx, "key")`
- Scalar API Reference (`r.Scalar`) — serves an interactive API reference UI from [Scalar](https://github.com/scalar/scalar) backed by an OpenAPI 3.1 document; opt-in via `SCALAR_ENABLED`; registers two routes: the docs UI (`SCALAR_DOCS_PATH`, default `/docs`) and the spec endpoint (`SCALAR_SPEC_PATH`, default `/openapi.json`); hybrid spec source: build programmatically via `api.Document` (set on `r.Scalar.Spec` or via `SetAPIDocument()`) or serve a static file (`SCALAR_SPEC_FILE`); configurable CDN URL (`SCALAR_CDN_URL`) for air-gapped use; wired in `routes.go` when enabled

### API Support

#### Scalar API Reference
- `r.Scalar` (type `ScalarConfig`) — serves the [Scalar](https://github.com/scalar/scalar) API reference UI from an OpenAPI 3.1 document; opt-in via `SCALAR_ENABLED`; wired in `routes.go` when enabled
- The `api/` subpackage provides: `api.Document` (OpenAPI 3.1 builder with fluent API), `api.Schema` (JSON Schema helpers + Go struct reflection), `api.Response` (standardized `{data, error, meta}` envelope), `api.OffsetPagination` / `api.CursorPagination` (query-param parsing + metadata generation)
- Response helpers on `Regius`: `WriteAPIResponse(w, status, data, meta...)` and `WriteAPIError(w, status, code, message, details...)` wrap the envelope and call `WriteJSON`
- CLI: `regius make api <name>` scaffolds a CRUD handler (`handlers/api_<name>.go`) with pagination, envelope, and routes-api.go mounting; also generates `handlers/api_<name>_doc.go` with an OpenAPI document builder (`<Name>APIDocument`) and auto-wires `a.App.Scalar.Spec` in `routes-api.go` (first handler sets the spec, subsequent handlers merge via `Spec.MergePaths`)
- `SCALAR_SHOW_CLIENTS` controls which client library code examples are shown in the Scalar UI; accepts a raw JS expression: `true` (show all), `["curl","fetch"]` (show only those), or `{"js":true,"shell":["curl"]}` (per-language); empty = show all (default); converted to Scalar's `hiddenClients` at runtime
- Configured via env vars in `regius.go` `New()`: `SCALAR_ENABLED`, `SCALAR_DOCS_PATH`, `SCALAR_SPEC_PATH`, `SCALAR_TITLE`, `SCALAR_CDN_URL`, `SCALAR_SPEC_FILE`, `SCALAR_THEME`, `SCALAR_SHOW_CLIENTS`

### Internationalization

#### Locale Detection & Middleware
- Locale resolution order: cookie (`LOCALE_COOKIE_NAME`, default `locale`) → `Accept-Language` header → `DEFAULT_LOCALE` (default `en`).
- Supported locales are configured via the comma-separated `SUPPORTED_LOCALES` env var (default `en,es`).
- The middleware is enabled by default and wired globally in `regius/routes.go`.
- Handlers can read the current locale with `i18n.Locale(ctx)` from `github.com/hbarral/regius/i18n`.

#### Translation Files
- Generated apps store translations under `locales/<code>/<code>.yaml` (e.g. `locales/en/en.yaml`).
- Files are embedded via `locales.Content` and loaded in `init.regius.go` with `i18n.LoadWithDefault(locales.Content, app.App.I18n.DefaultLocale)`.
- Translations follow the `ctxi18n` YAML/JSON format: the top-level key is the locale code, values are nested maps.
- Add a new locale with `./regius make locale <code>`, which creates `locales/<code>/<code>.yaml` seeded with the default translation keys.

#### Using Translations in Views
- **templ** (default renderer): import `github.com/hbarral/regius/i18n` and use `{ i18n.T(ctx, "key") }`. Interpolation: `i18n.T(ctx, "navbar.welcome", i18n.M{"name": userName})`.
- **jet** and **go** templates: use `{{T "key"}}` (the render package injects a `T` function that reads the locale from context).
- Always update the `<html lang="...">` attribute to use `i18n.Locale(ctx)` (templ) or `{{.Locale}}` (jet/go).

### Security Considerations

#### Encryption
- AES encryption for sensitive data
- Random string generation using crypto/rand

#### Input Validation
- `Validation` struct (`validator.go`, `validator_struct.go`, `validator_i18n.go`, `validator_middleware.go`) with error collection (`Errors` map + structured `Details`)
- Built-in rules: `Required`, `Check`, `IsEmail`, `IsURL`, `IsUUID`, `IsPhone`, `IsCreditCard`, `IsAlpha`, `IsAlphanumeric`, `IsNumeric`, `IsInt`, `IsFloat`, `IsDateISO`, `IsJSON`, `IsIP`, `IsBoolean`, `IsMinLength`, `IsMaxLength`, `IsLength`, `IsRange`, `NoSpaces`, `MatchesPattern`; uses `github.com/asaskevich/govalidator` + stdlib
- Custom rules: `Regius.RegisterValidation(name, fn)` registers reusable `ValidationFunc`s (built-ins pre-registered, overridable); invoke by name via `Validation.Rule(name, field, value, message...)`
- Struct validation: `Validation.ValidateStruct(s)` walks `validate` struct tags via reflection (`required`, `nested`, `field=`, `min=`, `max=`, `len=`, `range=N:M`, `oneof=`, `regex=`, registry rule names); nested structs/pointers/slices produce dot-path error keys (`Address.City`, `Items.0.Name`); optional rules skip empty values
- Localization: rule failures record i18n keys + params in `Details`; `LocalizedErrors(ctx)` translates via the `i18n` package for the request locale, falling back to English; 25 `validation.*` keys ship in the scaffolded locale files (`en`/`es`) and `cli/templates/locales/locale.yaml`
- Request validation middleware: `r.ValidateRequest(ValidationConfig{StructType, Rules, ErrorFormat, RedirectTo})` — JSON bodies decoded into `StructType` and tag-validated (retrievable via `regius.ValidatedFromContext[*T](ctx)`); form bodies validated field-by-field against `Rules`; failures respond with the API error envelope (default) or session flash + 303 redirect (`ErrorFormat: "form"`, errors readable via `PopValidationErrors`); gob-registers `map[string]string` for session storage; per-route middleware (not global)

#### CSRF Protection
- Automatic CSRF token generation and validation

#### Security Headers
- `r.SecurityHeaders(cfg)` sets a bundle of HTTP security response headers (CSP, HSTS, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, COOP, CORP, `X-Permitted-Cross-Domain-Policies`, `X-DNS-Prefetch-Control`)
- Helmet-style safe defaults are applied for any field left at its zero value
- HSTS (`Strict-Transport-Security`) is only emitted when `Server.Secure` is true, so it never locks out local dev
- Wired globally in `routes.go`; disabled (passthrough) when `SECURITY_HEADERS_ENABLED` is false/absent
- Configured via env vars in `regius.go` `New()`: `SECURITY_HEADERS_ENABLED`, `CONTENT_SECURITY_POLICY`, `HSTS_MAX_AGE`, `HSTS_INCLUDE_SUBDOMAINS`, `HSTS_PRELOAD`, `REFERRER_POLICY`, `X_FRAME_OPTIONS`
- A route can override any header per-response via `w.Header().Set(...)` before writing

#### API Key Authentication
- `r.APIKeyAuth(cfg)` authenticates requests via an API key for API route groups (e.g. `/api/*`); opt-in via `API_KEY_AUTH_ENABLED`. Not wired globally — apply it to a route group so cookie-authed web routes are unaffected
- Key sources (in order): `Authorization: Bearer <key>` (or configured scheme), `X-API-Key` header, and an opt-in query param (`API_KEY_QUERY_PARAM`, disabled by default since keys in URLs leak via logs/referrers)
- Validation backends by precedence: pluggable `Validator` func (DB-backed) > `APIKeyStore` (cache/revocation) > static `Keys` (constant-time compare via `crypto/subtle`)
- `CacheAPIKeyStore` adapts `cache.Cache` (Redis/Badger); entries are keyed by SHA-256 of the raw key so raw keys are never persisted; `Revoke` removes an entry
- On success the `APIKeyIdentity` is stored in the request context; retrieve it with `regius.APIKeyFromContext(ctx)`
- On failure responds `401` with `WWW-Authenticate: <scheme> realm="<realm>"`, `Cache-Control: no-store`, and a JSON body. The raw key is never logged
- Configured via env vars in `regius.go` `New()`: `API_KEY_AUTH_ENABLED`, `API_KEYS`, `API_KEY_HEADER`, `API_KEY_SCHEME`, `API_KEY_ALT_HEADER`, `API_KEY_QUERY_PARAM`, `API_KEY_REALM`

#### Request ID Tracing
- `r.RequestID(cfg)` stamps every request with a correlation ID; enabled by default (opt-out via `REQUEST_ID_ENABLED`)
- Reuses an incoming ID from the request header (`REQUEST_ID_HEADER`, default `X-Request-ID`) when present (cross-service correlation); otherwise generates one
- Generated ID format via `REQUEST_ID_FORMAT`: `uuid` (default) | `xid` | `short` (12-char base62) | `default` (chi-style `host/random-counter`); a pluggable `Generator` func overrides the format
- Echoed on the response via `REQUEST_ID_RESPONSE_HEADER` (default `X-Request-ID`); set before the downstream handler so a route can override per-response via `w.Header().Set(...)`
- Stored in the request context under chi's `RequestIDKey` (interoperable with `middleware.GetReqID` and chi's request logger, which prints the ID when `DEBUG=true`); retrieve via `regius.RequestIDFromContext(ctx)`
- Incoming IDs are trimmed and capped at 128 chars to prevent log injection / header abuse
- Wired globally in `routes.go` (replaces chi's built-in `middleware.RequestID`)
- Configured via env vars in `regius.go` `New()`: `REQUEST_ID_ENABLED`, `REQUEST_ID_HEADER`, `REQUEST_ID_RESPONSE_HEADER`, `REQUEST_ID_FORMAT`

#### Request Sanitization
- `r.RequestSanitizer(cfg)` sanitizes incoming request input (query params, form-encoded values, and a configurable header allowlist) for XSS prevention using [bluemonday](https://github.com/microcosm-cc/bluemonday); enabled in scaffolded apps via `REQUEST_SANITIZATION_ENABLED=true` (framework defaults to off when the env var is absent)
- Policies (env `REQUEST_SANITIZATION_POLICY`): `strict` (default — strips all HTML, returns safe text) | `ugc` (allows a safe HTML subset like `<b>`, `<a>`); a `Custom *bluemonday.Policy` field overrides both
- Scopes (all default to true via `*bool` fields — nil = true, `BoolPtr(false)` disables): `Query` sanitizes URL query params; `Form` sanitizes `application/x-www-form-urlencoded` and `multipart/form-data` text fields (in `r.Form`, `r.PostForm`, and `r.MultipartForm.Value`); `Headers` sanitizes only the names in the allowlist (`REQUEST_SANITIZATION_HEADERS`, default `Referer,User-Agent`)
- JSON-safe: `application/json` (and other non-form) bodies are **never** parsed or consumed — API handlers retain full access to `r.Body`
- Path exemption: `REQUEST_SANITIZATION_EXEMPT` (default `/api/.*`, mirroring `NoSurf`) bypasses sanitization for matching paths
- Non-destructive: clean values are left byte-for-byte intact; only values containing HTML are rewritten. `r.URL.RawQuery` is only re-encoded when a query value actually changed
- Standalone helpers: `r.Sanitize(s)` / `r.Sanitizer()` (app-configured policy) and package-level `regius.Sanitize(s)` (strict) for targeted use in handlers
- Do NOT sanitize structural headers (`Authorization`, `Cookie`, `X-CSRF-Token`, `Content-*`, `X-Forwarded-*`, `X-Request-ID`) — doing so breaks routing, auth, and tracing; the default allowlist avoids these
- Wired globally in `routes.go` (after `MaxRequestSize` so body parsing is bounded by the size limit, and after `NoSurf` so CSRF validation runs on raw input)
- Configured via env vars in `regius.go` `New()`: `REQUEST_SANITIZATION_ENABLED`, `REQUEST_SANITIZATION_POLICY`, `REQUEST_SANITIZATION_QUERY`, `REQUEST_SANITIZATION_FORM`, `REQUEST_SANITIZATION_HEADERS`, `REQUEST_SANITIZATION_EXEMPT`

#### IP Whitelist/Blacklist
- `r.IPFilter(cfg)` allows or denies requests based on the client IP; opt-in via `IP_FILTER_ENABLED`. Wired globally in `routes.go` immediately after `middleware.RealIP` so denied requests short-circuit before heavier middleware runs
- `Allow` and `Deny` accept IPs or CIDR ranges (e.g. `10.0.0.0/8`, `192.168.1.5`, `::1/128`); bare IPs are treated as `/32` (IPv4) or `/128` (IPv6). Invalid entries are logged via `ErrorLog` and skipped (non-fatal)
- Semantics are deny-wins: a matching `Deny` entry always blocks; when `Allow` is non-empty, any IP not in `Allow` is blocked. With neither list set, all IPs are allowed
- `TrustProxy` (default **false**) reads the client IP from `X-Forwarded-For` (first entry) or `X-Real-IP` instead of `RemoteAddr`. Only enable behind a trusted reverse proxy — otherwise an attacker can spoof the header to bypass the filter. Note `middleware.RealIP` already rewrites `RemoteAddr` from `X-Forwarded-For`, so the two interact; use `TrustProxy` for explicit control
- Blocked requests respond with `StatusCode` (default 403) + a JSON body (`{"error":"forbidden","message":...}`) and `Cache-Control: no-store`
- Optional pluggable `IPChecker` interface (`Check(ip) (IPDecision, error)`) for dynamic, DB/cache-backed decisions. `DecisionAllow`/`DecisionDeny` override the static lists; `DecisionNone` defers to them. Checker errors are logged and treated as `DecisionNone` (the dynamic layer fails open while the static baseline still applies)
- `CacheIPChecker` adapts `cache.Cache` (Redis/Badger) for runtime block/unblock without restart (fail2ban-style); entries namespaced under `ipfilter:`; `Block`/`Allow`/`Unblock`/`Set` with TTL in seconds
- Configured via env vars in `regius.go` `New()`: `IP_FILTER_ENABLED`, `IP_FILTER_ALLOW`, `IP_FILTER_DENY`, `IP_FILTER_TRUST_PROXY`, `IP_FILTER_STATUS_CODE`, `IP_FILTER_MESSAGE`

### CLI Commands Structure

#### Available Commands
```bash
./regius help                    # Show help
./regius version                 # Show version
./regius new <app_name>          # Create new app (defaults: --db sqlite --renderer templ)
                                 #   flags: --db <postgres|mysql|sqlite|...> (pre-fill .env DATABASE_TYPE; sqlite is default)
                                 #          --renderer <templ|jet|go> (templ is default)
                                 #          -v, --verbose (stream go get / go mod tidy output)
./regius migrate [up|down|reset] # Run migrations
./regius migrate version         # Show current migration version
./regius db:seed                 # Run pending SQL seed files
./regius make migration <name>          # Create SQL migration
./regius make seed <name>        # Create SQL seed file
./regius make auth               # Create auth scaffolding
./regius make handler <name>     # Create handler stub
./regius make model <name>       # Create model
./regius make gorm-model <name>  # Create GORM model
./regius make session            # Create session table
./regius make mail <name>        # Create mail templates
./regius make api <name>         # Create CRUD API handler with pagination + envelope
./regius make locale <code>      # Create a new translation locale (e.g. fr)
./regius down                    # Maintenance mode on
./regius up                      # Maintenance mode off
```

### Environment Configuration

#### Required Environment Variables
```bash
APP_NAME=testapp
APP_URL="http://localhost:4000"
DEBUG=true
PORT=4000
RPC_PORT=4001
SERVER_NAME=localhost
SECURE=false
DATABASE_TYPE=postgres|mysql|sqlite
DATABASE_HOST=localhost
DATABASE_PORT=5432
DATABASE_NAME=regius
DATABASE_USER=user
DATABASE_PASS=password
```

### Development Workflow

#### Creating New Features
1. Use CLI commands to scaffold code (`make handler`, `make model`, etc.)
2. Write tests first (table-driven where appropriate)
3. Implement functionality
4. Run `make test` to verify
5. Format code with `go fmt`
6. Build and test manually
7. **Check if the README needs updating** — when a feature adds user-facing behavior (new middleware, CLI command, config option, env var, or public API), update `README.md` (and `AGENTS.md`) to document it. The scaffolded `.env` template (`cmd/cli/templates/env`) must also be updated for any new env var so generated apps include it
8. **Update translations when adding user-facing strings** — if a feature adds or changes text shown in templates, add the new keys to `cmd/cli/_skeleton/locales/en/en.yaml` and `es/es.yaml`, plus `cmd/cli/templates/locales/locale.yaml` so newly generated locales inherit the keys

#### Code Generation
- Use the CLI's `make` commands for consistent scaffolding
- Follow existing patterns in generated code
- Customize generated code as needed

### Performance Considerations

#### Caching
- Redis cache support
- BadgerDB cache support
- In-memory caching

#### Database Connection Pooling
- Uses `database/sql` connection pooling
- Redis connection pooling

### Error Messages

#### User-Friendly Messages
- Use clear, descriptive error messages
- Avoid technical jargon in user-facing messages
- Provide actionable guidance when possible

#### Logging
- InfoLog and ErrorLog loggers available
- Use appropriate log levels
- Include context in log messages

### Dependencies

#### Core Dependencies
- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/alexedwards/scs/v2` - Session management
- `github.com/CloudyKit/jet/v6` - Template engine
- `github.com/gomodule/redigo/redis` - Redis client
- `github.com/dgraph-io/badger/v3` - Embedded key-value store

#### Development Dependencies
- Standard Go testing tools
- Consider adding `golangci-lint` for consistent linting

### Deployment

#### Binary Distribution
- Cross-platform binaries built via CI/CD
- Install to `~/.regius/bin/` directory
- Add to PATH for global access

#### Maintenance Mode
- `down` command puts app in maintenance mode
- `up` command brings app back online
- Affects all routes when active

This document should be updated as the codebase evolves. Run `go fmt ./...` and `make test` before committing changes.</content>
<parameter name="filePath">/home/hbarral/www/personal/regius/AGENTS.md
