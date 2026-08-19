package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var makeLocaleCmd = &cobra.Command{
	Use:   "locale [code]",
	Short: "Create a new locale translation file",
	Long: `Creates a new locale file under locales/<code>/<code>.yaml seeded with
all translation keys from the default application. Edit the values to translate
the application into the requested language.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeLocale(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Locale created!")
	},
}

func doMakeLocale(code string) error {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return errors.New("you must give the locale a code, e.g. fr")
	}

	localeDir := filepath.Join(b.RootPath, "locales", code)
	localeFile := filepath.Join(localeDir, code+".yaml")

	if fileExists(localeFile) {
		return fmt.Errorf("locale file already exists: %s", localeFile)
	}

	data, err := templateFS.ReadFile("templates/locales/locale.yaml")
	if err != nil {
		return err
	}

	content := strings.ReplaceAll(string(data), "${LOCALE}", code)
	content = strings.ReplaceAll(content, "${APP_NAME}", os.Getenv("APP_NAME"))

	if err := os.MkdirAll(localeDir, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(localeFile, []byte(content), 0644); err != nil {
		return err
	}

	return nil
}
