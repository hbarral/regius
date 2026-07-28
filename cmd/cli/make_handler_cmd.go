package main

import (
	"errors"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func init() {
	makeHandlerCmd.Flags().StringVar(&makeRenderer, "renderer", "", "template engine for the handler stub + view (templ|jet|go); defaults to RENDERER env or templ")
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

	renderer := strings.ToLower(resolveRenderer())
	if !validRenderers[renderer] {
		return errors.New("invalid renderer " + renderer + " (use templ|jet|go)")
	}

	lower := strings.ToLower(name)
	title := cases.Title(language.English, cases.NoLower).String(name)
	appName := os.Getenv("APP_NAME")

	fileName := reg.RootPath + "/handlers/" + lower + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	// handler stub
	data, err := templateFS.ReadFile("templates/handlers/handler." + handlerTemplateSuffix(renderer))
	if err != nil {
		return err
	}
	handler := string(data)
	handler = strings.ReplaceAll(handler, "$HANDLER_NAME", title)
	handler = strings.ReplaceAll(handler, "$VIEW_NAME", lower)
	handler = strings.ReplaceAll(handler, "${APP_NAME}", appName)
	if err := copyDataToFile([]byte(handler), fileName); err != nil {
		return err
	}

	// view stub
	stubPath, viewExt := handlerViewStub(renderer)
	viewData, err := templateFS.ReadFile(stubPath)
	if err == nil {
		view := string(viewData)
		view = strings.ReplaceAll(view, "$HANDLER_NAME", title)
		view = strings.ReplaceAll(view, "$VIEW_NAME", lower)
		view = strings.ReplaceAll(view, "${APP_NAME}", appName)
		viewFile := reg.RootPath + "/views/" + lower + viewExt
		if !fileExists(viewFile) {
			if err := copyDataToFile([]byte(view), viewFile); err != nil {
				return err
			}
		}
	}

	// regen templ sources if needed
	if renderer == "templ" {
		if err := runTemplGenerate(); err != nil {
			color.Yellow("  ! templ generate failed; run `templ generate` manually: %v", err)
		}
	}

	return nil
}
