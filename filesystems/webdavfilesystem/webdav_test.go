package webdavfilesystem

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/webdav"
)

const (
	testUser = "testuser"
	testPass = "testpass"
)

// newWebDAVTestServer starts an in-process WebDAV server backed by a temp dir
// and protected with HTTP Basic Auth.
func newWebDAVTestServer(t *testing.T, user, pass string) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()

	dav := &webdav.Handler{
		FileSystem: webdav.Dir(root),
		LockSystem: webdav.NewMemLS(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		dav.ServeHTTP(w, r)
	})

	return httptest.NewServer(mux), root
}

func webDAVClient(t *testing.T, srv *httptest.Server) *WebDAV {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return &WebDAV{
		Host:   u.Hostname(),
		Port:   u.Port(),
		User:   testUser,
		Pass:   testPass,
		UseSSL: false,
	}
}

func TestWebDAV_List(t *testing.T) {
	srv, root := newWebDAVTestServer(t, testUser, testPass)
	defer srv.Close()

	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello"), 0644))

	w := webDAVClient(t, srv)
	listing, err := w.List("/")

	require.NoError(t, err)
	require.Len(t, listing, 1)
	assert.Equal(t, "file.txt", listing[0].Key)
}

func TestWebDAV_List_Empty(t *testing.T) {
	srv, _ := newWebDAVTestServer(t, testUser, testPass)
	defer srv.Close()

	w := webDAVClient(t, srv)
	listing, err := w.List("/")

	require.NoError(t, err)
	assert.Empty(t, listing)
}

func TestWebDAV_List_Unauthorized(t *testing.T) {
	srv, _ := newWebDAVTestServer(t, testUser, testPass)
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	w := &WebDAV{
		Host:   u.Hostname(),
		Port:   u.Port(),
		User:   testUser,
		Pass:   "wrong-password",
		UseSSL: false,
	}

	_, err = w.List("/")
	assert.Error(t, err)
}

func TestWebDAV_Put(t *testing.T) {
	srv, root := newWebDAVTestServer(t, testUser, testPass)
	defer srv.Close()

	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("upload payload"), 0644))

	w := webDAVClient(t, srv)
	err := w.Put(localFile, "/")

	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(root, "upload.txt"))
	require.NoError(t, err)
	assert.Equal(t, "upload payload", string(content))
}

func TestWebDAV_Delete(t *testing.T) {
	srv, root := newWebDAVTestServer(t, testUser, testPass)
	defer srv.Close()

	require.NoError(t, os.WriteFile(filepath.Join(root, "delete-me.txt"), []byte("x"), 0644))

	w := webDAVClient(t, srv)
	ok := w.Delete([]string{"/delete-me.txt"})

	assert.True(t, ok)
	_, statErr := os.Stat(filepath.Join(root, "delete-me.txt"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestWebDAV_Delete_NotFound(t *testing.T) {
	srv, _ := newWebDAVTestServer(t, testUser, testPass)
	defer srv.Close()

	w := webDAVClient(t, srv)
	ok := w.Delete([]string{"/does-not-exist.txt"})

	assert.False(t, ok)
}

func TestWebDAV_Get(t *testing.T) {
	// Get is currently a no-op in this implementation.
	w := &WebDAV{}
	err := w.Get("/tmp", "/remote/file.txt")
	assert.NoError(t, err)
}
