package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHome(t *testing.T) {
	routes := getRoutes()
	testServer := httptest.NewServer(routes)
	defer testServer.Close()

	response, err := testServer.Client().Get(testServer.URL + "/")
	if err != nil {
		t.Log(err)
		t.Fatal(err)
	}

	if response.StatusCode != 200 {
		t.Errorf("expected status code 200, but got %d", response.StatusCode)
	}

	bodyText, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(bodyText), "Regius") {
		reg.TakeScreenShot(testServer.URL+"/", "TestHome", 1500, 1000)
		t.Errorf("expected body to contain 'Regius', but got %s", string(bodyText))
	}
}

func TestSetLanguage(t *testing.T) {
	routes := getRoutes()
	testServer := httptest.NewServer(routes)
	defer testServer.Close()

	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/set-language/es", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Referer", testServer.URL+"/")

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusSeeOther {
		t.Errorf("expected status code %d, but got %d", http.StatusSeeOther, response.StatusCode)
	}

	var found bool
	for _, c := range response.Cookies() {
		if c.Name == "locale" && c.Value == "es" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected locale cookie to be set to es")
	}
}
