package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var makeServiceCmd = &cobra.Command{
	Use:   "service [name]",
	Short: "Create a service-layer stub",
	Long: `Creates a business-logic service in the services directory, with its
shared dependencies (App, Models) injected and a Do stub to replace.

The first service also bootstraps the services hub (services/services.go),
adds the Services field to the application struct (main.go) and to the
handlers (handlers/handlers.go), and wires services.NewServices in
init.regius.go. Later services are appended to the hub. Handlers reach a
service via h.Services.<Name>.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeService(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Service created!")
	},
}

func doMakeService(name string) error {
	if name == "" {
		return errors.New("you must give the service a name")
	}

	// "billing-service" and "billing" both yield BillingService
	title := strings.TrimSuffix(pascalIdent(name), "Service")
	if title == "" {
		return errors.New("you must give the service a name")
	}
	typ := title + "Service"

	module := os.Getenv("APP_NAME")
	if module == "" {
		return errors.New("you must set APP_NAME in .env (the app's module name) to wire the services hub")
	}

	// the service file; refuse duplicates
	fileName := b.RootPath + "/services/" + snakeIdent(name) + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	data, err := templateFS.ReadFile("templates/services/service.go.tmpl")
	if err != nil {
		return err
	}
	src := string(data)
	src = strings.ReplaceAll(src, "$SERVICE_TYPE", typ)
	src = strings.ReplaceAll(src, "${APP_NAME}", module)
	if err := copyDataToFile([]byte(src), fileName); err != nil {
		return err
	}
	color.Yellow("  - Created services/%s.go", snakeIdent(name))

	hubPath := b.RootPath + "/services/services.go"
	if !fileExists(hubPath) {
		// first service: bootstrap the hub and wire it through the app
		if err := bootstrapServicesHub(hubPath, module, title, typ); err != nil {
			return err
		}
		color.Yellow("  - Created services/services.go (the services hub)")

		if err := wireServices(b.RootPath, module); err != nil {
			return err
		}
		color.Yellow("  - Added the Services field to the application struct (main.go) and the handlers (handlers/handlers.go)")
		color.Yellow("  - Wired services.NewServices(app.App, app.Models) in init.regius.go")
		color.Yellow("  - Next: reach the service from a handler via h.Services.%s", title)
	} else {
		if err := appendServiceToHub(hubPath, title, typ); err != nil {
			return err
		}
		color.Yellow("  - Appended %s to the services hub (services/services.go)", typ)
	}

	return nil
}

// bootstrapServicesHub writes the services hub carrying the first service.
func bootstrapServicesHub(hubPath, module, field, typ string) error {
	data, err := templateFS.ReadFile("templates/services/services.go.tmpl")
	if err != nil {
		return err
	}
	src := string(data)
	src = strings.ReplaceAll(src, "$FIELD_NAME", field)
	src = strings.ReplaceAll(src, "$SERVICE_TYPE", typ)
	src = strings.ReplaceAll(src, "${APP_NAME}", module)
	if err := copyDataToFile([]byte(src), hubPath); err != nil {
		return err
	}
	gofmtFile(hubPath)
	return nil
}

// appendServiceToHub adds one field + constructor entry to the services
// hub, keeping both markers last. Idempotent per service.
func appendServiceToHub(hubPath, field, typ string) error {
	fieldSnippet := fmt.Sprintf("\t%s *%s\n", field, typ)
	if err := insertAtMarker(hubPath, "// additional service fields are registered here", fieldSnippet,
		fmt.Sprintf("%s *%s", field, typ), nil); err != nil {
		return err
	}

	ctorSnippet := fmt.Sprintf("\t\t%s: New%s(app, models),\n", field, typ)
	if err := insertAtMarker(hubPath, "// additional service constructors are registered here", ctorSnippet,
		fmt.Sprintf("%s: New%s(app, models),", field, typ), nil); err != nil {
		return err
	}

	// normalize field alignment after the inserts
	gofmtFile(hubPath)
	return nil
}

// wireServices injects the services hub into the application struct, the
// handlers, and init.regius.go. Only runs on the first service; idempotent.
func wireServices(rootPath, module string) error {
	if err := wireServicesInMain(rootPath+"/main.go", module); err != nil {
		return err
	}
	if err := wireServicesInHandlers(rootPath+"/handlers/handlers.go", module); err != nil {
		return err
	}
	return wireServicesInInit(rootPath+"/init.regius.go", module)
}

func wireServicesInMain(path, module string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read main.go: %w", err)
	}
	str := addImport(string(data), "\""+module+"/services\"", "\""+module+"/middleware\"")

	field := "Services   *services.Services\n"
	newStr, ok := insertAfterLine(str, "Middleware *middleware.Middleware", "\t"+field)
	if !ok {
		newStr, ok = insertAfterLine(str, "*handlers.Handlers", "\t"+field)
	}
	if !ok {
		return fmt.Errorf("could not add the Services field to the application struct in main.go; add it manually:\n\n\tServices   *services.Services\n")
	}

	return writeFormatted(path, []byte(newStr))
}

func wireServicesInHandlers(path, module string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read handlers/handlers.go: %w", err)
	}
	str := addImport(string(data), "\""+module+"/services\"", "\""+module+"/data\"")

	field := "Services *services.Services\n"
	newStr, ok := insertAfterLine(str, "Models data.Models", "\t"+field)
	if !ok {
		return fmt.Errorf("could not add the Services field to the Handlers struct in handlers/handlers.go; add it manually:\n\n\tServices *services.Services\n")
	}

	return writeFormatted(path, []byte(newStr))
}

func wireServicesInInit(path, module string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read init.regius.go: %w", err)
	}
	str := string(data)

	if strings.Contains(str, "services.NewServices(app.App, app.Models)") {
		return nil // already wired
	}

	str = addImport(str, "\""+module+"/services\"", "\""+module+"/middleware\"")

	call := "app.Services = services.NewServices(app.App, app.Models)"
	if strings.Contains(str, "myHandlers") {
		call += "\n\tmyHandlers.Services = app.Services"
	}

	// marker (skeletons since the services scaffolding), else the workers
	// marker, else right before return app
	switch {
	case strings.Contains(str, "\t// register services here"):
		str = strings.Replace(str, "\t// register services here", "\t// register services here\n\n\t"+call, 1)
	case strings.Contains(str, "\t// register background workers here"):
		str = strings.Replace(str, "\t// register background workers here", "\t"+call+"\n\n\t// register background workers here", 1)
	case strings.Contains(str, "\n\treturn app"):
		str = strings.Replace(str, "\n\treturn app", "\n\t"+call+"\n\n\treturn app", 1)
	default:
		return fmt.Errorf("no services registration point found in init.regius.go; add before return app:\n\n\tapp.Services = services.NewServices(app.App, app.Models)\n\tmyHandlers.Services = app.Services\n")
	}

	return writeFormatted(path, []byte(str))
}
