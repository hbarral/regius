# Regius

<img
    style="display: block; 
           margin-left: auto;
           margin-right: auto;
           width: 40%;"
    src="dist/regius-app/public/images/regius.png" 
    alt="Regius Logo">
</img>

Regius is a CLI application for building web pages, inspired by Laravel but built with Go. It offers tools for database migrations and code generation, providing an agile and organized development experience.

## 🌍 Repository

Visit the official repository at [Regius on GitHub](https://github.com/hbarral/regius).

## 📋 Features

### Basic Commands

- `regius new <app_name>`: Creates a new web application.
- `regius version`: Print application version.
- `regius help`: Show help for any command.
- `regius up`: Bring the server back from maintenance mode.
- `regius down`: Put the server in maintenance mode.

### Migration Commands

- `regius migrate`: Run all pending migrations (defaults to "up").
- `regius migrate up`: Run all pending migrations.
- `regius migrate down [steps|all]`: Reverse migrations (use "all" for all migrations).
- `regius migrate reset`: Reset and re-run all migrations.

### Code Generation Commands

- `regius make migration <name> --format=<fizz|sql>`: Create migration files (default: fizz).
- `regius make auth`: Create authentication system (tables, models, middleware, handlers, views).
- `regius make handler <name>`: Create a handler stub.
- `regius make model <name>`: Create a new model with proper pluralization.
- `regius make session`: Create session table in database.
- `regius make key`: Generate 32-character encryption key.
- `regius make mail <name>`: Create mail templates.

### CLI Features

- **Automatic help**: `--help` flag on all commands and subcommands
- **Flag support**: Use `--format=fizz` instead of positional arguments
- **Shell completion**: Generate autocompletion scripts for bash, zsh, fish, and PowerShell
- **Better validation**: Improved argument validation and error messages
- **Command aliases**: Future support for command shortcuts

### Examples

```bash
# Create migration with fizz format (default)
regius make migration create_users --format=fizz

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
  - IP whitelisting: Exclude specific IPs from rate limiting
  - Proxy support: Trust X-Forwarded-For and X-Real-IP headers
  - Standard HTTP headers: X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Window, Retry-After
  - Per-path rate limiting: Each route path gets its own rate limit
  - Apply globally or to specific routes (API, auth, etc.)

  **Usage Example in Your App:**

  ```go
  // In regius-app/routes.go - Apply to all routes
  a.use(a.Middleware.RateLimit)

  // In regius-app/routes-api.go - Apply to API routes
  r.Use(a.Middleware.APIRateLimit)

  // Or apply to specific routes
  r.Post("/login", a.Middleware.RateLimitStrict(a.Handlers.Login))
  ```

  **Configuration Options:**

  ```go
  config := regius.RateLimiterConfig{
      Enabled:    true,                    // Enable/disable rate limiting
      Algorithm:  regius.RateLimiterAlgorithmSlidingWindow,  // "token_bucket" or "sliding_window"
      Requests:   100,                   // Maximum requests per window
      Window:     time.Minute,           // Time duration (time.Second, time.Minute, time.Hour)
      Storage:    "",                     // "" for in-memory, "redis" or "badger"
      TrustProxy: true,                   // Trust proxy headers
      Whitelist:  []string{"127.0.0.1", "::1"},  // IPs to exclude
  }
  ```

  **Testing:**
  The skeleton app includes comprehensive testing tools in `test-tools/` directory:

  - `ratelimit-test.py` - Python-based tester with detailed output
  - `ratelimit-test.sh` - Shell script using curl
  - `ratelimit-tester.go` - Go-based high-performance tester

  **Documentation:**

  - Full documentation: `regius/RATE_LIMITER.md`
  - Implementation details: `regius/RATE_LIMITER_IMPLEMENTATION.md`
  - Testing guide: `regius-app/test-tools/README.md`
  - Quick start: `regius-app/test-tools/QUICKSTART.md`

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
      Enabled:            true,                                              // Enable/disable CORS
      AllowedOrigins:     []string{"*"},                                     // Allowed origins (use "*" for any)
      AllowedMethods:     []string{"GET","POST","PUT","DELETE","OPTIONS"},   // Allowed HTTP methods
      AllowedHeaders:     []string{"Accept","Authorization","Content-Type"}, // Allowed request headers
      ExposedHeaders:     []string{},                                        // Headers exposed to the client
      MaxAge:             300,                                               // Preflight cache duration in seconds
      AllowCredentials:   true,                                              // Allow cookies/auth headers
      OptionsPassthrough: false,                                             // Let OPTIONS requests pass through
      Debug:              false,                                             // Enable debug logging
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
      Enabled:                       true,                          // Master toggle
      ContentSecurityPolicy:         "default-src 'self'",          // Empty -> default
      HSTSMaxAge:                    31536000,                       // 0 -> 1 year default
      HSTSIncludeSubDomains:         true,                           // Add includeSubDomains
      HSTSPreload:                   false,                          // Add preload
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
      Enabled:    true,                 // Master toggle
      Keys:       []string{"secret"},   // Static valid keys (constant-time compare)
      Validator:  nil,                  // Pluggable func(key) (identity, ok); takes precedence
      Store:      nil,                  // APIKeyStore (e.g. CacheAPIKeyStore) for lookup/revocation
      Header:     "Authorization",      // Primary header (default "Authorization")
      Scheme:     "Bearer",             // Scheme prefix for Header (default "Bearer")
      AltHeader:  "X-API-Key",          // Secondary header, no prefix (default "X-API-Key")
      QueryParam: "",                   // Opt-in query param name (default "" = off)
      Realm:      "api",                // Used in WWW-Authenticate (default "api")
  }

  // Cache-backed store (keys hashed with SHA-256, never stored raw):
  store := regius.NewCacheAPIKeyStore(a.Cache, "apikey:")
  _ = store.Set("client-secret", regius.APIKeyIdentity{ID: "client-1"}, 0)
  _ = store.Revoke("client-secret") // invalidate later
  ```

  **Environment Variables:**

  ```env
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
      Enabled:        true,                 // Master toggle (default true)
      Header:         "X-Request-ID",       // Request header to read incoming ID from
      ResponseHeader: "X-Request-ID",       // Response header to echo the ID on ("" = don't echo)
      Format:         regius.RequestIDFormatUUID, // "uuid" | "xid" | "short" | "default"
      Generator:      nil,                  // Optional override of Format
  }
  ```

  **Environment Variables:**

  ```env
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
      Enabled: true,                          // Master toggle
      Policy:  regius.SanitizePolicyStrict,   // "strict" (default) | "ugc"
      Query:   regius.BoolPtr(true),          // Sanitize URL query params (default true)
      Form:    regius.BoolPtr(true),          // Sanitize form-encoded values (default true)
      Headers: []string{"Referer", "User-Agent"}, // Header allowlist (default none)
      Exempt:  "/api/.*",                     // Regex of paths to skip (default "/api/.*")
      Custom:  nil,                           // Optional *bluemonday.Policy override
  }

  // BoolPtr is a tiny helper to set *bool fields (nil defaults to true):
  regius.BoolPtr(false) // explicitly disable a scope
  ```

  **Environment Variables:**

  ```env
  REQUEST_SANITIZATION_ENABLED=true
  REQUEST_SANITIZATION_POLICY=strict         # strict | ugc
  REQUEST_SANITIZATION_QUERY=true
  REQUEST_SANITIZATION_FORM=true
  REQUEST_SANITIZATION_HEADERS=Referer,User-Agent
  REQUEST_SANITIZATION_EXEMPT=/api/.*
  ```

## 🚀 Getting Started

### Download Binaries

Download the suitable binary for your operating system from the links below:

- [Linux](https://github.com/hbarral/regius/releases/download/v1.5.0/regius_Linux_x86_64.tar.gz)
- [Windows](https://github.com/hbarral/regius/releases/download/v1.5.0/regius_Windows_x86_64.zip)
- [Mac](https://github.com/hbarral/regius/releases/download/v1.5.0/regius_Darwin_x86_64.tar.gz)

<details>
  <summary>Build from Source</summary>

1. Clone the repository:

   ```bash
    git clone https://github.com/hbarral/regius.git
   cd regius
   ```

2. Build the project for your operating system:

   ```bash
   go build -o regius ./cmd/cli
   ```

3. Run the binary:

   ```bash
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

# database configuration
DATABASE_TYPE=
DATABASE_HOST=
# ...
```

<details>
  <summary>See the full .env example</summary>

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

# database configuration
DATABASE_TYPE=
DATABASE_HOST=
DATABASE_PORT=
DATABASE_USER=
DATABASE_PASS=
DATABASE_NAME=
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

# Template engine configuration (jet | go)
RENDERER=jet

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
CONTENT_SECURITY_POLICY=default-src 'self'
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

```
# database configuration
DATABASE_TYPE=postgres
DATABASE_HOST=127.0.0.1
DATABASE_PORT=5432
DATABASE_USER=postgres
DATABASE_PASS=postgres
DATABASE_NAME=myapp
DATABASE_SSL_MODE=disable
```

Fill in these values with your database connection details. Migrations use these environment variables directly - no additional configuration file required.

### Password Hashing

Regius provides a centralized password hashing utility accessible via `App.Hash`, supporting `bcrypt` (default), `scrypt`, and `argon2id`. The algorithm and its parameters are configured through environment variables:

```
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

  ```bash
  ./regius new myapp
  ```

- Show help commands:

  ```bash
  ./regius help
  ```

- Run a migration:

  ```bash
  ./regius migration
  ```

- Create a migration:

  ```bash
  ./regius make migration create_users_table fizz
  ```

- Create a model:

  ```bash
  ./regius make model User
  ```

- Put the server in maintenance mode:

  ```bash
  ./regius down
  ```

- Bring the server back from maintenance mode:
  ```bash
  ./regius up
  ```

For more details about usage and commands, refer to the CLI help:

```bash
./regius help
```

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
