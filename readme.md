# 🌟 Regius

Regius is a CLI application for building web pages, inspired by Laravel but built with Go. It offers tools for database migrations and code generation, providing an agile and organized development experience.

<p align="center">
  <img src="https://gitlab.com/hbarral/regius-app/-/raw/main/public/images/regius.png" width="50%" alt="Regius Logo">
</p>

## 🌍 Repository

Visit the official repository at [Regius on GitLab](https://gitlab.com/hbarral/regius).

## 📋 Features

- `new <app_name>`: Creates a new web application.
- `help`: Show the help commands.
- `version`: Print application version.
- `migration`: Runs all "up" migrations that have not been run previously.
- `migration down`: Reverses the most recent migration.
- `migration reset`: Runs all "down" migrations in reverse order, and then all "up" migrations.
- `make migration <name> <format>`: Creates two new migrations (up and down) in the migrations folder; format can be `fizz` or `sql`.
- `make auth`: Creates and runs migrations for authentication tables, and creates models and middleware.
- `make handler <name>`: Creates a stub handler in the handlers directory.
- `make model <name>`: Creates a new model in the data directory.
- `make session`: Creates a table in the database as session store.
- `make mail <name>`: Creates two starter mail templates in the mail directory.
- `down`: Put the server in maintenance mode.
- `up`: Bring the server back from maintenance mode.

## 🚀 Getting Started

### Download Binaries

Download the suitable binary for your operating system from the links below:

- [Linux](https://gitlab.com/hbarral/regius/-/jobs/artifacts/main/download?job=build_linux)
- [Windows](https://gitlab.com/hbarral/regius/-/jobs/artifacts/main/download?job=build_windows)
- [Mac](https://gitlab.com/hbarral/regius/-/jobs/artifacts/main/download?job=build_mac)

<details>
  <summary>Build from Source</summary>

  1. Clone the repository:
      ```bash
      git clone https://gitlab.com/hbarral/regius.git
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

Contributions are welcome! Please follow the GitLab flow for contributions:

1. Fork the project.
2. Create a new branch (`git checkout -b feature-new-feature`).
3. Make your changes and commit (`git commit -am 'Add new feature'`).
4. Push to the branch (`git push origin feature-new-feature`).
5. Open a Merge Request.

## 📄 License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.

---

*Made with ❤️ by the Regius team.*
