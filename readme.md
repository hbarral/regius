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

# types of files allowed to upload
ALLOWED_FILETYPES="image/png,image/jpeg,image/gif,application/pdf"
# 5MB
MAX_FILESIZE=5242880

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

After creating a new application, a `config/database.yml` file is generated. This file contains database configurations for different environments and is consumed by the migration generator. Below is an example:

```yaml
# config/database.yml
development:
  dialect:
  database:
  user:
  password:
  host:
  port:
  pool: 5
```

Fill in this file with your database connection details for the development environment, and similarly for production or other environments, if needed.

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
