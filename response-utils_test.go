package regius

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payload struct {
	Name string `json:"name" xml:"name"`
	Age  int    `json:"age" xml:"age"`
}

func TestReadJSON_Valid(t *testing.T) {
	e := &Regius{}

	body := strings.NewReader(`{"name":"alice","age":30}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	w := httptest.NewRecorder()

	var got payload
	err := e.ReadJSON(w, req, &got)

	require.NoError(t, err)
	assert.Equal(t, "alice", got.Name)
	assert.Equal(t, 30, got.Age)
}

func TestReadJSON_Malformed(t *testing.T) {
	e := &Regius{}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{bad json`))
	w := httptest.NewRecorder()

	var got payload
	err := e.ReadJSON(w, req, &got)

	assert.Error(t, err)
}

func TestReadJSON_MultipleValues(t *testing.T) {
	e := &Regius{}

	body := strings.NewReader(`{"name":"a","age":1}{"name":"b","age":2}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	w := httptest.NewRecorder()

	var got payload
	err := e.ReadJSON(w, req, &got)

	require.Error(t, err)
	assert.Equal(t, "body must only have a single json value", err.Error())
}

func TestReadJSON_TrailingWhitespaceOK(t *testing.T) {
	e := &Regius{}

	body := strings.NewReader(`{"name":"a","age":1}` + "\n  \t")
	req := httptest.NewRequest(http.MethodPost, "/", body)
	w := httptest.NewRecorder()

	var got payload
	err := e.ReadJSON(w, req, &got)

	require.NoError(t, err)
	assert.Equal(t, "a", got.Name)
}

func TestReadJSON_OversizeBody(t *testing.T) {
	e := &Regius{}

	// 1 MB + 1 byte exceeds the 1 MB (1048576) MaxBytesReader limit.
	body := strings.NewReader(strings.Repeat("x", 1048577))
	req := httptest.NewRequest(http.MethodPost, "/", body)
	w := httptest.NewRecorder()

	var got payload
	err := e.ReadJSON(w, req, &got)

	assert.Error(t, err)
}

func TestWriteJSON(t *testing.T) {
	r := &Regius{}

	w := httptest.NewRecorder()
	data := payload{Name: "alice", Age: 30}

	err := r.WriteJSON(w, http.StatusCreated, data)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got payload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, data, got)
}

func TestWriteJSON_CustomHeaders(t *testing.T) {
	r := &Regius{}

	w := httptest.NewRecorder()
	headers := http.Header{}
	headers.Set("X-Trace-Id", "abc123")

	err := r.WriteJSON(w, http.StatusOK, payload{Name: "x", Age: 1}, headers)
	require.NoError(t, err)

	assert.Equal(t, "abc123", w.Header().Get("X-Trace-Id"))
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestWriteXML(t *testing.T) {
	r := &Regius{}

	w := httptest.NewRecorder()
	data := payload{Name: "alice", Age: 30}

	err := r.WriteXML(w, http.StatusOK, data)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))

	var got payload
	require.NoError(t, xml.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, data, got)
}

func TestDownloadFile(t *testing.T) {
	c := &Regius{}

	dir := t.TempDir()
	content := []byte("file contents")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.txt"), content, 0644))

	req := httptest.NewRequest(http.MethodGet, "/download", nil)
	w := httptest.NewRecorder()

	err := c.DownloadFile(w, req, dir, "report.txt")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "report.txt")
	assert.Equal(t, content, w.Body.Bytes())
}

func TestError404(t *testing.T) {
	c := &Regius{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	c.Error404(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, http.StatusText(http.StatusNotFound)+"\n", w.Body.String())
}

func TestError500(t *testing.T) {
	c := &Regius{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	c.Error500(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestErrorUnauthorized(t *testing.T) {
	c := &Regius{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	c.ErrorUnauthorized(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestErrorForbidden(t *testing.T) {
	c := &Regius{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	c.ErrorForbidden(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestErrorStatus(t *testing.T) {
	r := &Regius{}

	tests := []int{
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusTeapot,
	}

	for _, status := range tests {
		t.Run(http.StatusText(status), func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ErrorStatus(w, status)

			assert.Equal(t, status, w.Code)
			assert.Equal(t, http.StatusText(status)+"\n", w.Body.String())
		})
	}
}
