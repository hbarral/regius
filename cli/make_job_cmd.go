package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var makeJobCmd = &cobra.Command{
	Use:   "job [name]",
	Short: "Create a background job",
	Long: `Creates a background job in the workers directory: a typed payload, a
handler with TODO markers, and an enqueue helper.

On the first run it also bootstraps the scaffolding around it: the
workers/register.go hub, the RegisterAll wiring in init.regius.go, and the
regius_jobs/regius_locks table migration for the DATABASE_TYPE dialect
(delete the migration if JOBS_BACKEND stays memory or redis).

Jobs run when JOBS_ENABLED=true; set JOBS_BACKEND=sql (then run
./regius migrate) or redis to persist work beyond the process. Delivery is
at-least-once: keep handlers idempotent. Recurring schedules can be added
with Manager.Cron/Every from RegisterAll.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeJob(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Background job created!")
	},
}

func doMakeJob(name string) error {
	if name == "" {
		return errors.New("you must give the job a name")
	}

	lower := strings.ToLower(name)
	title := webhookTitle(lower)
	jobName := strings.ReplaceAll(lower, "-", "_")

	// the handler file; refuse duplicates
	fileName := b.RootPath + "/workers/" + jobName + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	data, err := templateFS.ReadFile("templates/workers/job.go.tmpl")
	if err != nil {
		return err
	}
	src := string(data)
	src = strings.ReplaceAll(src, "$HANDLER_NAME", title)
	src = strings.ReplaceAll(src, "$JOB_NAME", jobName)
	if err := copyDataToFile([]byte(src), fileName); err != nil {
		return err
	}

	color.Yellow("  - Created workers/%s.go (job name: %s)", jobName, jobName)

	registerPath := b.RootPath + "/workers/register.go"
	if !fileExists(registerPath) {
		// first job: bootstrap the hub, wire RegisterAll into
		// init.regius.go, and scaffold the jobs table migration
		regData, err := templateFS.ReadFile("templates/workers/register.go.tmpl")
		if err != nil {
			return err
		}
		regSrc := string(regData)
		regSrc = strings.ReplaceAll(regSrc, "$HANDLER_NAME", title)
		regSrc = strings.ReplaceAll(regSrc, "$JOB_NAME", jobName)
		if err := copyDataToFile([]byte(regSrc), registerPath); err != nil {
			return err
		}
		color.Yellow("  - Created workers/register.go")

		dialect, up, down, err := jobsTableMigration()
		if err != nil {
			return err
		}
		if err := b.CreateMigration(up, down, "create_regius_jobs_table", "sql"); err != nil {
			return err
		}
		color.Yellow("  - Created migrations/..._create_regius_jobs_table.up/down.sql (%s); delete it if JOBS_BACKEND stays memory or redis", dialect)

		if err := wireWorkersRegistration(b.RootPath+"/init.regius.go", os.Getenv("APP_NAME")); err != nil {
			return err
		}
		color.Yellow("  - Wired workers.RegisterAll(app.App.Jobs) in init.regius.go")
	} else {
		// later jobs: append the registration to the hub
		if err := appendRegistration(registerPath, jobName, title); err != nil {
			return err
		}
	}

	color.Yellow("  - Next: set JOBS_ENABLED=true in .env (and JOBS_BACKEND=sql + ./regius migrate, or JOBS_BACKEND=redis) to run them")

	return nil
}

// jobsTableMigration loads the regius_jobs/regius_locks DDL for the app's
// DATABASE_TYPE dialect (defaulting to sqlite when unset).
func jobsTableMigration() (dialect string, up, down []byte, err error) {
	dialect = normalizeDBType(b.DBType)
	if dialect == "" {
		dialect = "sqlite"
	}
	switch dialect {
	case "postgres", "mysql", "sqlite":
	default:
		return "", nil, nil, fmt.Errorf("unsupported DATABASE_TYPE %q for the jobs table migration", b.DBType)
	}

	up, err = templateFS.ReadFile("templates/migrations/jobs_table." + dialect + ".up.sql")
	if err != nil {
		return "", nil, nil, err
	}
	down, err = templateFS.ReadFile("templates/migrations/jobs_table." + dialect + ".down.sql")
	if err != nil {
		return "", nil, nil, err
	}
	return dialect, up, down, nil
}

// appendRegistration adds one MustRegister entry to the workers hub,
// keeping the marker comment last. Idempotent per job name.
func appendRegistration(registerPath, jobName, title string) error {
	data, err := os.ReadFile(registerPath)
	if err != nil {
		return fmt.Errorf("failed to read workers/register.go: %w", err)
	}

	str := string(data)
	if strings.Contains(str, fmt.Sprintf("m.MustRegister(%q, %s,", jobName, title)) {
		return nil
	}

	snippet := fmt.Sprintf("\tm.MustRegister(%q, %s, jobs.Options{\n\t\tMaxAttempts: 3, // TODO: tune per job\n\t})\n", jobName, title)
	marker := "// additional jobs are registered here"
	if strings.Contains(str, "\n\t"+marker) {
		str = strings.Replace(str, "\n\t"+marker, snippet+"\n\t"+marker, 1)
	} else {
		// fallback: before the closing brace of RegisterAll (the last one
		// in the file)
		idx := strings.LastIndex(str, "\n}")
		if idx < 0 {
			return errors.New("no registration insertion point found in workers/register.go; add the MustRegister line manually")
		}
		str = str[:idx] + "\n" + snippet + str[idx:]
	}

	if err := os.WriteFile(registerPath, []byte(str), 0644); err != nil {
		return fmt.Errorf("failed to write workers/register.go: %w", err)
	}
	return nil
}

// wireWorkersRegistration inserts the workers import and the
// RegisterAll(app.App.Jobs) call into init.regius.go, at the marker comment
// or (fallback) right before "return app". Idempotent.
func wireWorkersRegistration(initPath, module string) error {
	if module == "" {
		return errors.New("you must set APP_NAME in .env (the app's module name) to wire the workers import")
	}

	data, err := os.ReadFile(initPath)
	if err != nil {
		return fmt.Errorf("failed to read init.regius.go: %w", err)
	}
	str := string(data)

	call := "workers.RegisterAll(app.App.Jobs)"
	if strings.Contains(str, call) {
		return nil // already wired
	}

	str = addWorkersImport(str, module)

	marker := "// register background workers here"
	if strings.Contains(str, marker) {
		str = strings.Replace(str, "\t"+marker, "\t"+marker+"\n\n\t"+call, 1)
	} else {
		if !strings.Contains(str, "\n\treturn app") {
			return errors.New("no workers registration point found in init.regius.go; add workers.RegisterAll(app.App.Jobs) before return app")
		}
		str = strings.Replace(str, "\n\treturn app", "\n\n\t"+call+"\n\n\treturn app", 1)
	}

	if err := os.WriteFile(initPath, []byte(str), 0644); err != nil {
		return fmt.Errorf("failed to write init.regius.go: %w", err)
	}
	return nil
}

// addWorkersImport ensures the app module's workers package is imported,
// preferring the spot right after the middleware import (the skeleton
// layout) and falling back to the import block's closing paren.
func addWorkersImport(src, module string) string {
	imp := "\"" + module + "/workers\""
	if strings.Contains(src, imp) {
		return src
	}

	anchor := "\"" + module + "/middleware\""
	if strings.Contains(src, anchor) {
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
