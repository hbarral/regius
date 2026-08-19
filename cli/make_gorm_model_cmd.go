package cli

import (
	"errors"
	"strings"

	"github.com/fatih/color"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"
)

func init() {
	makeCmd.AddCommand(makeGormModelCmd)
}

var makeGormModelCmd = &cobra.Command{
	Use:   "gorm-model [name]",
	Short: "Create a new GORM model",
	Long:  `Creates a new GORM model in the data directory with proper pluralization.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeGormModel(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("GORM model created!")
	},
}

func doMakeGormModel(name string) error {
	if name == "" {
		return errors.New("you must give the model a name")
	}

	data, err := templateFS.ReadFile("templates/data/gorm-model")
	if err != nil {
		return err
	}

	model := string(data)
	plur := pluralize.NewClient()

	modelName := name
	tableName := name

	if plur.IsPlural(name) {
		modelName = plur.Singular(name)
		tableName = strings.ToLower(tableName)
	} else {
		tableName = strings.ToLower(plur.Plural(name))
	}

	fileName := b.RootPath + "/data/" + strings.ToLower(modelName) + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	model = strings.ReplaceAll(model, "$MODEL_NAME", strcase.ToCamel(modelName))
	model = strings.ReplaceAll(model, "$TABLE_NAME", tableName)

	return copyDataToFile([]byte(model), fileName)
}
