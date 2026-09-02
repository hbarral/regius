package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var makeAPICmd = &cobra.Command{
	Use:   "api [name]",
	Short: "Create a CRUD API handler",
	Long: `Creates a JSON API handler with CRUD endpoints (list, get, create, update, delete)
in the handlers directory, and mounts it in routes-api.go.

Also generates an OpenAPI documentation file (handlers/api_<name>_doc.go) with a
function that builds an api.Document for the handler's endpoints.

The handler uses the standard API response envelope (WriteAPIResponse / WriteAPIError)
and includes offset-based pagination on the list endpoint.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeAPI(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("API handler created!")
	},
}

func doMakeAPI(name string) error {
	if name == "" {
		return errors.New("you must give the API resource a name")
	}

	lower := strings.ToLower(name)
	title := cases.Title(language.English, cases.NoLower).String(name)
	appName := os.Getenv("APP_NAME")

	fileName := b.RootPath + "/handlers/api_" + lower + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	data, err := templateFS.ReadFile("templates/handlers/api-handler.go.tmpl")
	if err != nil {
		return err
	}

	handler := string(data)
	handler = strings.ReplaceAll(handler, "$HANDLER_NAME", title)
	handler = strings.ReplaceAll(handler, "${APP_NAME}", appName)
	handler = strings.ReplaceAll(handler, "$resource_name", lower)

	if err := copyDataToFile([]byte(handler), fileName); err != nil {
		return err
	}

	docFileName := b.RootPath + "/handlers/api_" + lower + "_doc.go"
	docData, err := templateFS.ReadFile("templates/handlers/api-handler-doc.go.tmpl")
	if err != nil {
		return err
	}

	doc := string(docData)
	doc = strings.ReplaceAll(doc, "$HANDLER_NAME", title)
	doc = strings.ReplaceAll(doc, "${APP_NAME}", appName)
	doc = strings.ReplaceAll(doc, "$resource_name", lower)

	if err := copyDataToFile([]byte(doc), docFileName); err != nil {
		return err
	}

	routesAPIPath := b.RootPath + "/routes-api.go"
	routesData, err := os.ReadFile(routesAPIPath)
	if err != nil {
		return fmt.Errorf("failed to read routes-api.go: %w", err)
	}

	routesStr := string(routesData)
	mountLine := fmt.Sprintf(`r.Mount("/%s", a.Handlers.%sRoutes())`, lower, title)

	docCall := fmt.Sprintf(`a.Handlers.%sAPIDocument(a.App.Server.URL)`, title)
	specLine := fmt.Sprintf("\t\ta.App.Scalar.Spec = %s", docCall)
	if strings.Contains(routesStr, "a.App.Scalar.Spec = a.Handlers.") {
		specLine = fmt.Sprintf("\t\ta.App.Scalar.Spec.MergePaths(%s)", docCall)
	}

	if err := insertRoutesBlock(routesAPIPath, mountLine+"\n"+specLine); err != nil {
		return err
	}

	color.Yellow("  - Created handlers/api_%s.go with CRUD endpoints", lower)
	color.Yellow("  - Created handlers/api_%s_doc.go with OpenAPI documentation", lower)
	color.Yellow("  - Mounted /api/%s in routes-api.go", lower)
	color.Yellow("  - Wired Scalar spec (enable SCALAR_ENABLED=true and visit /docs)")
	color.Yellow("  - Uses api.Response envelope + offset pagination on list endpoint")

	return nil
}
