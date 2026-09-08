package cli

import (
	"errors"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var makeResourceCmd = &cobra.Command{
	Use:   "resource [name]",
	Short: "Create an API resource (JSON transformer)",
	Long: `Creates an API resource in the resources directory: a struct that shapes
a model for API responses, with a constructor for one item and one for a
collection. Handlers pass the result to h.App.WriteAPIResponse so every
endpoint answers with the same {data, error, meta} envelope.

Use "regius make api <name> --with-resource" to generate the CRUD endpoints
and the resource together.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeResource(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Resource created!")
	},
}

func doMakeResource(name string) error {
	if name == "" {
		return errors.New("you must give the resource a name")
	}

	if err := generateResourceFile(name); err != nil {
		return err
	}

	color.Yellow("  - Created resources/%s_resource.go", snakeIdent(name))
	color.Yellow("  - Next: map your model's fields in New%s(s), then pass them to WriteAPIResponse from your handlers", pascalIdent(name)+"Resource")

	return nil
}

// generateResourceFile writes resources/<name>_resource.go, the JSON
// transformer for one model. Refuses duplicates. Shared with
// "regius make api --with-resource".
func generateResourceFile(name string) error {
	lower := strings.ToLower(name)
	title := pascalIdent(name)

	fileName := b.RootPath + "/resources/" + snakeIdent(name) + "_resource.go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	data, err := templateFS.ReadFile("templates/resources/resource.go.tmpl")
	if err != nil {
		return err
	}

	src := string(data)
	// the plural token must be replaced first: it has the singular one as
	// its prefix
	src = strings.ReplaceAll(src, "$RESOURCE_NAME_PLURAL", title+"Resources")
	src = strings.ReplaceAll(src, "$RESOURCE_NAME", title+"Resource")
	src = strings.ReplaceAll(src, "$MODEL_NAME", title)
	src = strings.ReplaceAll(src, "$resource_name", lower)
	return copyDataToFile([]byte(src), fileName)
}
