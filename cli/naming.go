package cli

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// pascalIdent converts a CLI resource name into an exported Go identifier:
// "send-welcome-email", "send_welcome_email", and "sendWelcomeEmail" all
// become "SendWelcomeEmail". Word separators (- and _) are normalized to
// spaces first because cases.Title keeps hyphens, which are invalid in Go
// identifiers.
func pascalIdent(name string) string {
	words := strings.NewReplacer("-", " ", "_", " ").Replace(strings.ToLower(name))
	title := cases.Title(language.English, cases.NoLower).String(words)
	return strings.ReplaceAll(title, " ", "")
}

// snakeIdent converts a CLI resource name into a lower_snake_case
// identifier: "send-welcome-email" becomes "send_welcome_email".
func snakeIdent(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "-", "_")
}

// envIdent converts a CLI resource name into an UPPER_SNAKE_CASE string for
// environment variable names: "stripe-payment" becomes "STRIPE_PAYMENT".
func envIdent(name string) string {
	return strings.ToUpper(snakeIdent(name))
}
