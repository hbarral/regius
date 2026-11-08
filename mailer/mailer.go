package mailer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	apimail "github.com/ainsleyclark/go-mail"
	premailer "github.com/vanng822/go-premailer/premailer"
	mail "github.com/xhit/go-simple-mail/v2"
)

type Mail struct {
	Domain      string
	Templates   string
	Host        string
	Port        int
	Username    string
	Password    string
	Encryption  string
	FromAddress string
	FromName    string
	Jobs        chan Message
	Results     chan Result
	API         string
	APIKey      string
	APIUrl      string
}

type Message struct {
	From        string
	FromName    string
	To          string
	Subject     string
	Template    string
	Attachments []string
	Data        interface{}
}

type Result struct {
	Success bool
	Error   error
}

// Account describes the configuration for a single mail account. It is used
// to register named accounts (e.g. "no-reply", "info") in addition to the
// default account. Every account carries its own transport settings, so each
// can use a different SMTP server, API provider, or From address.
type Account struct {
	Name        string `json:"name"`
	Domain      string `json:"domain"`
	Templates   string `json:"templates"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Encryption  string `json:"encryption"`
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
	API         string `json:"api"`
	APIKey      string `json:"api_key"`
	APIUrl      string `json:"api_url"`
}

// ToMail converts the account into a ready-to-use Mail. When the account does
// not define a templates directory, templatesDir is used.
func (a Account) ToMail(templatesDir string) *Mail {
	tpl := a.Templates
	if tpl == "" {
		tpl = templatesDir
	}
	return &Mail{
		Domain:      a.Domain,
		Templates:   tpl,
		Host:        a.Host,
		Port:        a.Port,
		Username:    a.Username,
		Password:    a.Password,
		Encryption:  a.Encryption,
		FromAddress: a.FromAddress,
		FromName:    a.FromName,
		Jobs:        make(chan Message, 20),
		Results:     make(chan Result, 20),
		API:         a.API,
		APIKey:      a.APIKey,
		APIUrl:      a.APIUrl,
	}
}

func (m *Mail) ListenForMail() {
	for {
		msg := <-m.Jobs
		err := m.Send(msg)
		if err != nil {
			m.Results <- Result{false, err}
		} else {
			m.Results <- Result{true, nil}
		}
	}
}

// InheritFrom fills zero-valued transport settings from base. The account
// identity fields (Username, Password, FromAddress) are deliberately left
// alone, so a named account can be a different mailbox on the same server while
// reusing the default host/port/encryption/API configuration.
func (m *Mail) InheritFrom(base Mail) {
	if m.Domain == "" {
		m.Domain = base.Domain
	}
	if m.Host == "" {
		m.Host = base.Host
	}
	if m.Port == 0 {
		m.Port = base.Port
	}
	if m.Encryption == "" {
		m.Encryption = base.Encryption
	}
	if m.FromName == "" {
		m.FromName = base.FromName
	}
	if m.API == "" {
		m.API = base.API
	}
	if m.APIKey == "" {
		m.APIKey = base.APIKey
	}
	if m.APIUrl == "" {
		m.APIUrl = base.APIUrl
	}
}

func (m *Mail) ChooseAPI(msg Message) error {
	switch m.API {
	case "mailgun", "sparkpost", "sendgrid":
		return m.SendUsingAPI(msg, m.API)
	default:
		return fmt.Errorf("unknown api %s; only mailgun, sparkpost, sendgrid supported", m.API)
	}
}

func (m *Mail) SendUsingAPI(msg Message, transport string) error {
	if msg.From == "" {
		msg.From = m.FromAddress
	}

	if msg.FromName == "" {
		msg.FromName = m.FromName
	}

	cfg := apimail.Config{
		URL:         m.APIUrl,
		APIKey:      m.APIKey,
		Domain:      m.Domain,
		FromAddress: msg.From,
		FromName:    msg.FromName,
	}

	driver, err := apimail.NewClient(transport, cfg)
	if err != nil {
		return err
	}

	formattedMessage, err := m.buildHTMLMessage(msg)
	if err != nil {
		return err
	}

	plainMessage, err := m.buildPlainTextMessage(msg)
	if err != nil {
		return err
	}

	tx := &apimail.Transmission{
		Recipients: []string{msg.To},
		Subject:    msg.Subject,
		HTML:       formattedMessage,
		PlainText:  plainMessage,
	}

	err = m.addAPIAttachments(msg, tx)
	if err != nil {
		return err
	}

	_, err = driver.Send(tx)
	if err != nil {
		return err
	}

	return nil
}

func (m *Mail) addAPIAttachments(msg Message, tx *apimail.Transmission) error {
	if len(msg.Attachments) > 0 {
		var attachments []apimail.Attachment

		for _, x := range msg.Attachments {
			var attach apimail.Attachment
			content, err := os.ReadFile(x)
			if err != nil {
				return err
			}

			fileName := filepath.Base(x)
			attach.Bytes = content
			attach.Filename = fileName
			attachments = append(attachments, attach)
		}

		tx.Attachments = attachments
	}

	return nil
}

func (m *Mail) Send(msg Message) error {
	if len(m.API) > 0 && len(m.APIKey) > 0 && len(m.APIUrl) > 0 && m.API != "smtp" {
		return m.ChooseAPI(msg)
	}
	return m.SendSMTPMessage(msg)
}

func (m *Mail) SendSMTPMessage(msg Message) error {
	formattedMessage, err := m.buildHTMLMessage(msg)
	if err != nil {
		return err
	}

	plainMessage, err := m.buildPlainTextMessage(msg)
	if err != nil {
		return err
	}

	server := mail.NewSMTPClient()
	server.Host = m.Host
	server.Port = m.Port
	server.Username = m.Username
	server.Password = m.Password
	server.Encryption = m.getEncryption(m.Encryption)
	server.KeepAlive = false
	server.ConnectTimeout = 10 * time.Second
	server.SendTimeout = 10 * time.Second

	smtpClient, err := server.Connect()
	if err != nil {
		return err
	}

	email := mail.NewMSG()
	email.
		SetFrom(msg.From).
		AddTo(msg.To).
		SetSubject(msg.Subject)

	email.SetBody(mail.TextHTML, formattedMessage)
	email.AddAlternative(mail.TextPlain, plainMessage)

	if len(msg.Attachments) > 0 {
		for _, x := range msg.Attachments {
			email.AddAttachment(x)
		}
	}

	err = email.Send(smtpClient)
	if err != nil {
		return err
	}

	return nil
}

func (m *Mail) getEncryption(e string) mail.Encryption {
	switch e {
	case "tls":
		return mail.EncryptionSTARTTLS
	case "ssl":
		return mail.EncryptionSSL
	case "none":
		return mail.EncryptionNone
	default:
		return mail.EncryptionSTARTTLS
	}
}

func (m *Mail) buildHTMLMessage(msg Message) (string, error) {
	templateToRender := fmt.Sprintf("%s/%s.html.template", m.Templates, msg.Template)

	t, err := template.New("email-html").ParseFiles(templateToRender)
	if err != nil {
		return "", err
	}

	var tpl bytes.Buffer
	if err = t.ExecuteTemplate(&tpl, "body", msg.Data); err != nil {
		return "", err
	}

	formattedMessage := tpl.String()
	formattedMessage, err = m.inlineCSS(formattedMessage)
	if err != nil {
		return "", err
	}

	return formattedMessage, nil
}

func (m *Mail) buildPlainTextMessage(msg Message) (string, error) {
	templateToRender := fmt.Sprintf("%s/%s.plain.template", m.Templates, msg.Template)

	t, err := template.New("email-html").ParseFiles(templateToRender)
	if err != nil {
		return "", err
	}

	var tpl bytes.Buffer
	if err = t.ExecuteTemplate(&tpl, "body", msg.Data); err != nil {
		return "", err
	}

	plainMessage := tpl.String()

	return plainMessage, nil
}

func (m *Mail) inlineCSS(s string) (string, error) {
	options := premailer.Options{
		RemoveClasses:     false,
		CssToAttributes:   false,
		KeepBangImportant: true,
	}

	prem, err := premailer.NewPremailerFromString(s, &options)
	if err != nil {
		return "", err
	}

	html, err := prem.Transform()
	if err != nil {
		return "", err
	}

	return html, nil
}

// Manager is a registry of named mail accounts. It holds the default account
// (also exposed via Regius.Mail for backward compatibility) plus any number of
// additional named accounts, each with its own transport configuration and its
// own buffered Jobs/Results channels.
type Manager struct {
	defaultName string
	accounts    map[string]*Mail
}

// NewManager creates an empty mail account manager.
func NewManager() *Manager {
	return &Manager{accounts: make(map[string]*Mail)}
}

// Register adds a named account. Names are case-insensitive; the first account
// registered becomes the default. Registering a duplicate name returns an error.
func (m *Manager) Register(name string, mail *Mail) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return fmt.Errorf("mailer: account name cannot be empty")
	}
	if mail == nil {
		return fmt.Errorf("mailer: account %q cannot be nil", name)
	}
	if _, ok := m.accounts[name]; ok {
		return fmt.Errorf("mailer: account %q already registered", name)
	}
	if m.defaultName == "" {
		m.defaultName = name
	}
	m.accounts[name] = mail
	return nil
}

// Account returns the named account. An empty name returns the default account.
func (m *Manager) Account(name string) (*Mail, error) {
	if name == "" {
		name = m.defaultName
	}
	if name == "" {
		return nil, fmt.Errorf("mailer: no default account configured")
	}
	mail, ok := m.accounts[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("mailer: account %q not configured", name)
	}
	return mail, nil
}

// Default returns the default account, or nil when no accounts are configured.
func (m *Manager) Default() *Mail {
	if m.defaultName == "" {
		return nil
	}
	return m.accounts[m.defaultName]
}

// Accounts returns a copy of the registered accounts keyed by name.
func (m *Manager) Accounts() map[string]*Mail {
	out := make(map[string]*Mail, len(m.accounts))
	for k, v := range m.accounts {
		out[k] = v
	}
	return out
}

// Start launches the background mail worker for every registered account.
func (m *Manager) Start() {
	for _, mail := range m.accounts {
		go mail.ListenForMail()
	}
}
