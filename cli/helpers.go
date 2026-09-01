package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// migrationDSN returns a DSN suitable for golang-migrate.
func migrationDSN() (string, error) {
	return b.MigrationDSNForCLI()
}

// defaultRegiusVersion is the go.mod pin used when the CLI's Version is unset
// (local `go build`, where Version is "dev"). Release builds set Version via
// goreleaser ldflags; this only matters for local dev, where `go get` (run
// right after go.mod is written) bumps it to latest and a `replace` directive
// overrides it entirely.
const defaultRegiusVersion = "v1.9.2"

// regiusGoModVersion returns the github.com/hbarral/regius version to pin in a
// generated app's go.mod: the CLI's own release Version, falling back to
// defaultRegiusVersion for local builds. A leading "v" is ensured because
// goreleaser's {{.Version}} strips it while go.mod requires it.
func regiusGoModVersion() string {
	v := Version
	if v == "" || v == "dev" {
		return defaultRegiusVersion
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// normalizeDBType maps the user-facing DATABASE_TYPE aliases (sqlite3,
// postgresql, mariadb) to the canonical template-file suffix used across the
// embedded migration templates (sqlite, postgres, mysql).
func normalizeDBType(dbType string) string {
	switch strings.ToLower(dbType) {
	case "sqlite3":
		return "sqlite"
	case "postgresql":
		return "postgres"
	case "mariadb":
		return "mysql"
	default:
		return strings.ToLower(dbType)
	}
}

func updateSourceFiles(path string, fi os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if fi.IsDir() {
		return nil
	}

	// Rewrite the literal module name `regius-app` to the chosen app name in
	// Go sources, templ sources, and templ-generated sources so imports resolve
	// after the app is renamed. Generated templ code only references the app
	// module via imports of the app's own `views` packages.
	matchedGo, _ := filepath.Match("*.go", fi.Name())
	matchedTempl, _ := filepath.Match("*.templ", fi.Name())

	if !(matchedGo || matchedTempl) {
		return nil
	}

	read, err := os.ReadFile(path)
	if err != nil {
		exitGracefully(err)
	}

	newContents := strings.ReplaceAll(string(read), "regius-app", appURL)

	if err := os.WriteFile(path, []byte(newContents), 0o644); err != nil {
		exitGracefully(err)
	}

	return nil
}

func updateSource() {
	err := filepath.Walk(".", updateSourceFiles)
	if err != nil {
		exitGracefully(err)
	}
}

func exitGracefully(err error, msg ...string) {
	if err != nil {
		exitWithError(err)
	}

	if len(msg) > 0 {
		exitWithSuccess(msg[0])
	}

	exitWithSuccess("Finished!")
}

func checkForDB() {
	if b.DBType == "" {
		exitGracefully(errors.New("you must set DATABASE_TYPE in .env"))
	}

	// SQLite uses a local file path (DATABASE_NAME, optional) and does not
	// need a network host, port, or database name.
	if normalizeDBType(b.DBType) == "sqlite" {
		return
	}

	if os.Getenv("DATABASE_HOST") == "" {
		exitGracefully(errors.New("DATABASE_HOST must be set"))
	}

	if os.Getenv("DATABASE_NAME") == "" {
		exitGracefully(errors.New("DATABASE_NAME must be set"))
	}
}

// insertRoutesBlock inserts block (one or more tab-indented lines) into the
// app's routes-api.go, inside the /api route group. The block is inserted at
// the "// add any API route here" marker when present, otherwise right after
// the r.Route("/api", ...) opening line. Idempotent: returns nil without
// writing when the block is already mounted.
func insertRoutesBlock(routesPath, block string) error {
	routesData, err := os.ReadFile(routesPath)
	if err != nil {
		return fmt.Errorf("failed to read routes-api.go: %w", err)
	}

	routesStr := string(routesData)
	firstLine := strings.SplitN(block, "\n", 2)[0]
	if strings.Contains(routesStr, firstLine) {
		return nil
	}

	if strings.Contains(routesStr, "func(_ chi.Router)") {
		routesStr = strings.Replace(routesStr, "func(_ chi.Router)", "func(r chi.Router)", 1)
	}

	marker := "// add any API route here"
	if strings.Contains(routesStr, marker) {
		routesStr = strings.Replace(routesStr, marker, block+"\n\n\t\t"+marker, 1)
	} else {
		// Fallback: insert right after the first r.Route(...) opening line.
		// The generated skeleton uses r.Route("/", ...) here (it is mounted
		// at /api from routes.go), so matching a hardcoded path would fail.
		openIdx := strings.Index(routesStr, "r.Route(")
		if openIdx < 0 {
			return errors.New("no route insertion point found in routes-api.go")
		}
		lineEnd := strings.Index(routesStr[openIdx:], "\n")
		if lineEnd < 0 {
			routesStr += "\n\t\t" + block + "\n"
		} else {
			pos := openIdx + lineEnd
			routesStr = routesStr[:pos] + "\n\t\t" + block + routesStr[pos:]
		}
	}

	if err := os.WriteFile(routesPath, []byte(routesStr), 0644); err != nil {
		return fmt.Errorf("failed to write routes-api.go: %w", err)
	}

	return nil
}

// appendEnvVar adds name=value to the app's .env under a generated-by
// comment. It never overwrites: if the variable is already defined (on any
// line, commented or not), the file is left untouched so re-running a make
// command stays idempotent.
func appendEnvVar(path, name, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read .env: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == name || strings.HasPrefix(trimmed, name+"=") {
			return nil
		}
	}

	entry := fmt.Sprintf("\n# generated by `regius make webhook`\n%s=%s\n", name, value)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open .env: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to update .env: %w", err)
	}

	return nil
}
