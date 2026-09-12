package cli

import (
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
)

// addImport ensures imp ("module/pkg") is present in src's import block,
// inserting it right after the anchor import when present and otherwise
// before the block's closing paren. No-op when already imported.
func addImport(src, imp, anchor string) string {
	if strings.Contains(src, imp) {
		return src
	}

	if anchor != "" && strings.Contains(src, anchor) {
		return strings.Replace(src, anchor, anchor+"\n\t"+imp, 1)
	}

	if i := strings.Index(src, "import ("); i >= 0 {
		if j := strings.Index(src[i:], "\n)"); j > 0 {
			pos := i + j
			return src[:pos] + "\n\t" + imp + src[pos:]
		}
	}
	return src
}

// insertAfterLine inserts newLine (a full line including its trailing
// newline) right after the first line containing contains. Returns ok=false
// when no such line exists.
func insertAfterLine(content, contains, newLine string) (string, bool) {
	idx := strings.Index(content, contains)
	if idx < 0 {
		return "", false
	}
	end := strings.IndexByte(content[idx:], '\n')
	if end < 0 {
		return content + newLine, true
	}
	pos := idx + end + 1
	return content[:pos] + newLine + content[pos:], true
}

// writeFormatted writes content to path, normalizing it with gofmt when it
// parses (so injected fields align with existing ones); when it does not
// parse the raw content is written as-is.
func writeFormatted(path string, content []byte) error {
	if out, err := format.Source(content); err == nil {
		content = out
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// gofmtFile normalizes an existing file with gofmt, leaving it untouched
// when it does not parse or cannot be read.
func gofmtFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if out, err := format.Source(data); err == nil {
		_ = os.WriteFile(path, out, 0644)
	}
}

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

// insertAtMarker inserts snippet (a full line-oriented block ending in a
// newline) into the file at path, immediately before the line containing
// marker, keeping that marker line last. already is the substring used for
// the idempotency check: when present in the file, nothing is written. When
// no line contains marker, fallback derives the insertion point from the
// file contents and returns the modified content, or ok=false when none
// exists.
func insertAtMarker(path, marker, snippet, already string, fallback func(content string) (string, bool)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filepath.Base(path), err)
	}

	content := string(data)
	if already != "" && strings.Contains(content, already) {
		return nil
	}

	if line, ok := markerLine(content, marker); ok {
		content = strings.Replace(content, line, snippet+"\n"+line, 1)
	} else {
		if fallback == nil {
			return fmt.Errorf("no insertion point found in %s (missing %q marker)", filepath.Base(path), marker)
		}
		content, ok = fallback(content)
		if !ok {
			return fmt.Errorf("no insertion point found in %s (missing %q marker)", filepath.Base(path), marker)
		}
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filepath.Base(path), err)
	}

	return nil
}

// markerLine returns the whole line containing marker (including its
// indentation and trailing newline), if marker is present in content.
func markerLine(content, marker string) (string, bool) {
	idx := strings.Index(content, marker)
	if idx < 0 {
		return "", false
	}
	start := strings.LastIndex(content[:idx], "\n") + 1
	end := strings.IndexByte(content[idx:], '\n')
	if end < 0 {
		return content[start:], true
	}
	return content[start : idx+end+1], true
}

// indentLines prefixes every non-empty line of block with indent.
func indentLines(block, indent string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

// insertRoutesBlock inserts block (one or more lines, unindented) into the
// app's routes-api.go, inside the /api route group, with the group's
// two-tab indentation. The block is inserted at the "// add any API route
// here" marker when present, otherwise right after the r.Route("/api", ...)
// opening line. Idempotent: returns nil without writing when the block is
// already mounted.
func insertRoutesBlock(routesPath, block string) error {
	// generated mount lines reference the chi router param, so make sure the
	// route-group closure's param is named
	if err := nameChiRouterParam(routesPath); err != nil {
		return err
	}

	snippet := indentLines(block, "\t\t") + "\n"
	firstLine := strings.SplitN(block, "\n", 2)[0]
	return insertAtMarker(routesPath, "// add any API route here", snippet, firstLine, func(content string) (string, bool) {
		// Fallback: insert right after the first r.Route(...) opening line.
		// The generated skeleton uses r.Route("/", ...) here (it is mounted
		// at /api from routes.go), so matching a hardcoded path would fail.
		openIdx := strings.Index(content, "r.Route(")
		if openIdx < 0 {
			return "", false
		}
		lineEnd := strings.Index(content[openIdx:], "\n")
		if lineEnd < 0 {
			return content + "\n" + snippet, true
		}
		pos := openIdx + lineEnd
		return content[:pos] + "\n" + snippet + content[pos:], true
	})
}

// nameChiRouterParam rewrites the skeleton's unnamed route-group closure
// param (func(_ chi.Router)) to a named one so generated mount lines
// compile. No-op when the param is already named.
func nameChiRouterParam(routesPath string) error {
	data, err := os.ReadFile(routesPath)
	if err != nil {
		return fmt.Errorf("failed to read routes-api.go: %w", err)
	}

	str := string(data)
	if !strings.Contains(str, "func(_ chi.Router)") {
		return nil
	}

	str = strings.Replace(str, "func(_ chi.Router)", "func(r chi.Router)", 1)
	if err := os.WriteFile(routesPath, []byte(str), 0644); err != nil {
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
