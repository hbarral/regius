package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// mailAccount mirrors the framework's mailer.Account shape without importing
// the root module (the CLI module stays dependency-minimal on purpose). Empty
// fields are omitted from the generated JSON so the framework can inherit the
// default account's transport settings at runtime.
type mailAccount struct {
	Name        string `json:"name"`
	Domain      string `json:"domain,omitempty"`
	Templates   string `json:"templates,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        int    `json:"port,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	Encryption  string `json:"encryption,omitempty"`
	FromAddress string `json:"from_address,omitempty"`
	FromName    string `json:"from_name,omitempty"`
	API         string `json:"api,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	APIUrl      string `json:"api_url,omitempty"`
}

var (
	makeMailAccountFrom       string
	makeMailAccountFromName   string
	makeMailAccountDomain     string
	makeMailAccountHost       string
	makeMailAccountUsername   string
	makeMailAccountPassword   string
	makeMailAccountEncryption string
	makeMailAccountAPI        string
	makeMailAccountAPIKey     string
	makeMailAccountAPIURL     string
	makeMailAccountPort       int
)

func init() {
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountFrom, "from", "", "From address (default: <name>@<MAIL_DOMAIN>)")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountFromName, "from-name", "", "From display name (default: inherit FROM_NAME)")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountDomain, "domain", "", "mail domain (default: inherit from the default account)")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountHost, "host", "", "SMTP host (default: inherit from the default account)")
	makeMailAccountCmd.Flags().IntVar(&makeMailAccountPort, "port", 0, "SMTP port (default: inherit from the default account)")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountUsername, "username", "", "SMTP username for this account")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountPassword, "password", "", "SMTP password for this account")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountEncryption, "encryption", "", "SMTP encryption: tls | ssl | none (default: inherit)")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountAPI, "api", "", "API provider (mailgun | sparkpost | sendgrid)")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountAPIKey, "api-key", "", "API key for the API provider")
	makeMailAccountCmd.Flags().StringVar(&makeMailAccountAPIURL, "api-url", "", "API URL for the API provider")
}

var makeMailAccountCmd = &cobra.Command{
	Use:   "mail-account [name]",
	Short: "Add a named mail account to .env",
	Long: `Adds a named mail account (e.g. no-reply, info, billing) to the MAIL_ACCOUNTS
JSON array in .env. Each account is an independent mailer with its own
credentials and From address; omitted transport settings (host, port,
encryption, API) are inherited from the default account at runtime.

Examples:
  regius make mail-account no-reply
  regius make mail-account info --from info@example.com --username info --password secret
  regius make mail-account billing --api sendgrid --api-key KEY --api-url https://api.sendgrid.com

Send with:
  mail, err := app.MailAccount("info")
  mail.Jobs <- mailer.Message{...}`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := doMakeMailAccount(args[0]); err != nil {
			exitGracefully(err)
		}
		color.Green("Mail account added!")
	},
}

func doMakeMailAccount(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return errors.New("you must give the mail account a name (e.g. no-reply, info)")
	}
	if strings.ContainsAny(name, "'\"\n") {
		return errors.New("mail account name must not contain quotes or newlines")
	}

	domain := makeMailAccountDomain
	if domain == "" {
		domain = os.Getenv("MAIL_DOMAIN")
	}
	if domain == "" && os.Getenv("APP_NAME") != "" {
		domain = os.Getenv("APP_NAME") + ".com"
	}

	from := makeMailAccountFrom
	if from == "" {
		if domain == "" {
			domain = "example.com"
		}
		from = name + "@" + domain
	}

	account := mailAccount{
		Name:        name,
		Domain:      makeMailAccountDomain,
		Host:        makeMailAccountHost,
		Port:        makeMailAccountPort,
		Username:    makeMailAccountUsername,
		Password:    makeMailAccountPassword,
		Encryption:  makeMailAccountEncryption,
		FromAddress: from,
		FromName:    makeMailAccountFromName,
		API:         makeMailAccountAPI,
		APIKey:      makeMailAccountAPIKey,
		APIUrl:      makeMailAccountAPIURL,
	}

	if err := appendMailAccount(b.RootPath+"/.env", account); err != nil {
		return err
	}

	color.Yellow("  - Added account %q to MAIL_ACCOUNTS in .env", name)
	color.Yellow("  - From address: %s", from)
	color.Yellow("  - Use it with: app.MailAccount(%q)", name)

	return nil
}

// appendMailAccount merges a named account into the MAIL_ACCOUNTS JSON array in
// .env. It preserves existing entries and refuses to overwrite an account that
// is already configured (idempotent re-runs fail with a clear message).
func appendMailAccount(path string, account mailAccount) error {
	accounts, idx, err := readMailAccounts(path)
	if err != nil {
		return err
	}

	for _, existing := range accounts {
		if strings.EqualFold(existing.Name, account.Name) {
			return fmt.Errorf("mail account %q is already configured in MAIL_ACCOUNTS", account.Name)
		}
	}

	accounts = append(accounts, account)
	encoded, err := json.Marshal(accounts)
	if err != nil {
		return fmt.Errorf("failed to encode MAIL_ACCOUNTS: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read .env: %w", err)
	}
	lines := strings.Split(string(data), "\n")

	line := "MAIL_ACCOUNTS='" + string(encoded) + "'"
	if idx >= 0 {
		lines[idx] = line
	} else {
		lines = append(lines, "", "# additional named mail accounts (JSON array)", line)
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to update .env: %w", err)
	}

	return nil
}

// readMailAccounts parses the current MAIL_ACCOUNTS value from .env and returns
// the accounts plus the line index of the MAIL_ACCOUNTS entry (or -1 when the
// key is absent).
func readMailAccounts(path string) ([]mailAccount, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, -1, fmt.Errorf("failed to read .env: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	idx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "MAIL_ACCOUNTS=") {
			idx = i
			break
		}
	}

	var accounts []mailAccount
	if idx < 0 {
		return accounts, -1, nil
	}

	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[idx]), "MAIL_ACCOUNTS="))
	raw = strings.Trim(raw, `"'`)
	if raw == "" {
		return accounts, idx, nil
	}

	if err := json.Unmarshal([]byte(raw), &accounts); err != nil {
		return nil, -1, fmt.Errorf("MAIL_ACCOUNTS in .env is not valid JSON: %w", err)
	}

	return accounts, idx, nil
}
