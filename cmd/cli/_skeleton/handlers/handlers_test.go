package handlers

import (
	"io"
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
