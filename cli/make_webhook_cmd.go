package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// webhookTitle converts a lowercased webhook name into the exported Go
// identifier prefix used in the generated handler, e.g. "stripe-payment"
// becomes "StripePayment" (cases.Title keeps hyphens, which are invalid in
// Go identifiers, so word separators are normalized to spaces first).
func webhookTitle(lower string) string {
	words := strings.NewReplacer("-", " ", "_", " ").Replace(lower)
	title := cases.Title(language.English, cases.NoLower).String(words)
	return strings.ReplaceAll(title, " ", "")
}

// webhookProviders maps the --provider flag value to the webhook preset
// function baked into the generated handler.
var webhookProviders = map[string]string{
	"generic": "Generic",
	"github":  "GitHub",
	"stripe":  "Stripe",
}

var makeWebhookProvider string

func init() {
	makeWebhookCmd.Flags().StringVar(&makeWebhookProvider, "provider", "generic", "webhook provider preset (generic|stripe|github)")
}

var makeWebhookCmd = &cobra.Command{
	Use:   "webhook [name]",
	Short: "Create an inbound webhook endpoint",
	Long: `Creates a signed inbound webhook endpoint in the handlers directory and
mounts it in routes-api.go at /api/webhooks/<name> (POST only).

The endpoint verifies the payload's HMAC signature with the framework's
webhook package before the body is parsed. The signing secret is read from
WEBHOOK_<NAME>_SECRET, which is appended to .env with a generated value.

Providers (--provider):
  generic  plain HMAC digest of the body in X-Signature (hex) [default]
  github   X-Hub-Signature-256 ("sha256=<hex>")
  stripe   Stripe-Signature ("t=<unix>,v1=<sig>") with 5m replay tolerance`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeWebhook(args[0], makeWebhookProvider); err != nil {
			exitGracefully(err)
		}
		color.Green("Webhook endpoint created!")
	},
}

func doMakeWebhook(name, provider string) error {
	if name == "" {
		return errors.New("you must give the webhook a name")
	}

	provider = strings.ToLower(provider)
	if provider == "" {
		provider = "generic"
	}
	preset, ok := webhookProviders[provider]
	if !ok {
		return fmt.Errorf("invalid provider %q (use generic|stripe|github)", provider)
	}

	lower := strings.ToLower(name)
	title := webhookTitle(lower)
	secretEnv := "WEBHOOK_" + strings.ToUpper(strings.ReplaceAll(lower, "-", "_")) + "_SECRET"

	fileName := b.RootPath + "/handlers/webhook_" + lower + ".go"
	if fileExists(fileName) {
		return errors.New(fileName + " already exists!")
	}

	data, err := templateFS.ReadFile("templates/handlers/webhook-handler.go.tmpl")
	if err != nil {
		return err
	}

	handler := string(data)
	handler = strings.ReplaceAll(handler, "$HANDLER_NAME", title)
	handler = strings.ReplaceAll(handler, "$PRESET_FUNC", preset)
	handler = strings.ReplaceAll(handler, "$PROVIDER_NAME", provider)
	handler = strings.ReplaceAll(handler, "$SECRET_ENV", secretEnv)
	handler = strings.ReplaceAll(handler, "$resource_name", lower)

	if err := copyDataToFile([]byte(handler), fileName); err != nil {
		return err
	}

	mountLine := fmt.Sprintf(`r.Mount("/webhooks/%s", a.Handlers.%sWebhookRoutes())`, lower, title)
	if err := insertRoutesBlock(b.RootPath+"/routes-api.go", mountLine); err != nil {
		return err
	}

	secret := b.RandomString(32)
	if err := appendEnvVar(b.RootPath+"/.env", secretEnv, secret); err != nil {
		return err
	}

	color.Yellow("  - Created handlers/webhook_%s.go (provider: %s)", lower, provider)
	color.Yellow("  - Mounted /api/webhooks/%s in routes-api.go (POST only)", lower)
	color.Yellow("  - Added %s to .env", secretEnv)
	color.Yellow("  - To expose it at /webhooks/%s instead, remount in routes-api.go and extend the NoSurf/sanitizer exemptions", lower)

	return nil
}
