package regius

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRodBrowser launches a headless Chromium browser for Dusk-style end-to-end
// tests. It skips the test when Chromium is not installed.
func newRodBrowser(t *testing.T) *rod.Browser {
	t.Helper()

	bin := "chromium"
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("browser %q not available: %v", bin, err)
	}

	l := launcher.New().
		Bin(bin).
		Headless(true).
		NoSandbox(true).
		Devtools(false)

	url, err := l.Launch()
	require.NoError(t, err)
	t.Cleanup(func() { l.Kill() })

	browser := rod.New().ControlURL(url)
	require.NoError(t, browser.Connect())
	t.Cleanup(func() { _ = browser.Close() })

	return browser
}

func TestDusk_Navigation(t *testing.T) {
	r := newTestApp(t, nil)
	r.Routes.Get("/dusk", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Dusk E2E</title></head><body><h1>Hello Dusk</h1></body></html>`)
	})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	browser := newRodBrowser(t)
	page, err := browser.Page(proto.TargetCreateTarget{URL: ts.URL + "/dusk"})
	require.NoError(t, err)
	require.NoError(t, page.WaitLoad())

	titleEl, err := page.Element("title")
	require.NoError(t, err)
	title, err := titleEl.Text()
	require.NoError(t, err)
	assert.Equal(t, "Dusk E2E", title)

	h1, err := page.Element("h1")
	require.NoError(t, err)
	text, err := h1.Text()
	require.NoError(t, err)
	assert.Equal(t, "Hello Dusk", text)
}

func TestDusk_FormSubmission(t *testing.T) {
	var submitted string

	r := newTestApp(t, nil)
	r.Routes.Get("/form", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>`+
			`<form method="POST" action="/api/submit">`+
			`<input name="name" id="name">`+
			`<button type="submit" id="btn">Submit</button>`+
			`</form></body></html>`)
	})
	r.Routes.Post("/api/submit", func(w http.ResponseWriter, req *http.Request) {
		submitted = req.FormValue("name")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<html><body><h1>Hello %s</h1></body></html>", submitted)
	})

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	browser := newRodBrowser(t)
	page, err := browser.Page(proto.TargetCreateTarget{URL: ts.URL + "/form"})
	require.NoError(t, err)
	require.NoError(t, page.WaitLoad())

	nameInput, err := page.Element("#name")
	require.NoError(t, err)
	require.NoError(t, nameInput.Input("Dusk"))

	btn, err := page.Element("#btn")
	require.NoError(t, err)
	require.NoError(t, btn.Click(proto.InputMouseButtonLeft, 1))

	require.NoError(t, page.WaitLoad())

	h1, err := page.Element("h1")
	require.NoError(t, err)
	text, err := h1.Text()
	require.NoError(t, err)
	assert.Equal(t, "Hello Dusk", text)
	assert.Equal(t, "Dusk", submitted)
}
