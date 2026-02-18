package main

import (
	"errors"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	// This will be called from make_cmd.go
}

var makeMailCmd = &cobra.Command{
	Use:   "mail [name]",
	Short: "Create mail templates",
	Long:  `Creates two starter mail templates in the mail directory.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeMail(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Mail templates created!")
	},
}

func doMakeMail(name string) error {
	if name == "" {
		return errors.New("you must give the mail template a name")
	}

	htmlMail := reg.RootPath + "/mail/" + strings.ToLower(name) + ".html.template"
	plainMail := reg.RootPath + "/mail/" + strings.ToLower(name) + ".plain.template"

	err := copyFileFromTemplate("templates/mailer/mail.html.template", htmlMail)
	if err != nil {
		return err
	}

	err = copyFileFromTemplate("templates/mailer/mail.plain.template", plainMail)
	if err != nil {
		return err
	}

	return nil
}
