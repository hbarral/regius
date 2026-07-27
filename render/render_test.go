package render

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRender_Page(t *testing.T) {
	testRenderer.RootPath = "./testdata"

	tests := []struct {
		name          string
		view          Template
		errorExpected bool
		errorMessage  string
	}{
		{"go_page", testRenderer.Go("home"), false, "Error rendering Go template"},
		{"go_page_no_template", testRenderer.Go("no-file"), true, "No error rendering non-existent Go template, when one is expected"},
		{"jet_page", testRenderer.Jet("home", nil), false, "Error rendering Jet template"},
		{"jet_page_no_template", testRenderer.Jet("no-file", nil), true, "No error rendering non-existent Jet template, when one is expected"},
	}

	for _, e := range tests {
		r, err := http.NewRequest("GET", "/some-url", nil)
		if err != nil {
			t.Error(err)
		}

		w := httptest.NewRecorder()

		var modifiedReq *http.Request
		handler := testRenderer.Session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			modifiedReq = r
		}))
		handler.ServeHTTP(httptest.NewRecorder(), r)
		if modifiedReq != nil {
			r = modifiedReq
		}

		err = testRenderer.Page(w, r, e.view, nil)
		if e.errorExpected {
			if err == nil {
				t.Errorf("%s: %s", e.name, e.errorMessage)
			}
		} else {
			if err != nil {
				t.Errorf("%s: %s: %s", e.name, e.errorMessage, err.Error())
			}
		}
	}
}

func TestRender_GoPage(t *testing.T) {
	testRenderer.RootPath = "./testdata"

	w := httptest.NewRecorder()

	r, err := http.NewRequest("GET", "/url", nil)
	if err != nil {
		t.Error(err)
	}

	var modifiedReq *http.Request
	handler := testRenderer.Session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modifiedReq = r
	}))
	handler.ServeHTTP(httptest.NewRecorder(), r)
	if modifiedReq != nil {
		r = modifiedReq
	}

	err = testRenderer.Page(w, r, testRenderer.Go("home"), nil)
	if err != nil {
		t.Error("Error rendering page", err)
	}
}

func TestRender_JetPage(t *testing.T) {
	w := httptest.NewRecorder()

	r, err := http.NewRequest("GET", "/url", nil)
	if err != nil {
		t.Error(err)
	}

	var modifiedReq *http.Request
	handler := testRenderer.Session.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modifiedReq = r
	}))
	handler.ServeHTTP(httptest.NewRecorder(), r)
	if modifiedReq != nil {
		r = modifiedReq
	}

	err = testRenderer.Page(w, r, testRenderer.Jet("home", nil), nil)
	if err != nil {
		t.Error("Error rendering page", err)
	}
}
