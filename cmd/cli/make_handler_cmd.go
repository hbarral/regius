package main

import (
	"errors"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func init() {
	// This will be called from make_cmd.go
}

var makeHandlerCmd = &cobra.Command{
	Use:   "handler [name]",
	Short: "Create a handler stub",
	Long:  `Creates a stub handler in the handlers directory.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeHandler(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Handler created!")
	},
}

func doMakeHandler(name string) error {
	if name == "" {
		return errors.New("you must give the handler a name")
	}

	fileName := reg.RootPath + "/handlers/" + strings.ToLower(name) + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	data, err := templateFS.ReadFile("templates/handlers/handler")
	if err != nil {
		return err
	}

	handler := string(data)
	handler = strings.ReplaceAll(handler, "$HANDLER_NAME", cases.Title(language.English, cases.NoLower).String(name))

	return copyDataToFile([]byte(handler), fileName)
}
