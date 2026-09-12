//go:build !integration

package mailer

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// The mailer integration tests require Docker (via dockertest) to run
	// a MailHog container. Build with -tags integration to enable them.
	code := m.Run()
	os.Exit(code)
}
