package regius

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hbarral/regius/filesystems"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePutCall struct {
	fileName string
	folder   string
}

// fakeFS implements filesystems.FS for upload tests. Only Put is exercised.
type fakeFS struct {
	putCalls []fakePutCall
	putErr   error
}

func (f *fakeFS) Put(fileName, folder string) error {
	f.putCalls = append(f.putCalls, fakePutCall{fileName: fileName, folder: folder})
	return f.putErr
}

func (f *fakeFS) Get(string, ...string) error                { return nil }
func (f *fakeFS) List(string) ([]filesystems.Listing, error) { return nil, nil }
func (f *fakeFS) Delete([]string) bool                       { return false }

// pngSignature is the 8-byte PNG magic that mimetype detects as "image/png".
var pngSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// chdirToTemp changes the process working directory to a fresh temp dir and
// restores it on cleanup. Required because getFileToUpload writes to the
// hardcoded relative path "./tmp/...".
func chdirToTemp(t *testing.T) string {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

func newMultipartUploadRequest(t *testing.T, fieldName, fileName string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(fieldName, fileName)
	require.NoError(t, err)
	_, err = io.Copy(part, bytes.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestInSlice(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		s     string
		want  bool
	}{
		{"found", []string{"image/png", "image/jpeg"}, "image/png", true},
		{"not found", []string{"image/png", "image/jpeg"}, "image/gif", false},
		{"empty slice", []string{}, "image/png", false},
		{"nil slice", nil, "image/png", false},
		{"empty string present", []string{""}, "", true},
		{"empty string absent", []string{"a"}, "", false},
		{"case sensitive", []string{"Image/PNG"}, "image/png", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, inSlice(tt.slice, tt.s))
		})
	}
}

func TestGetFileToUpload_Success(t *testing.T) {
	chdirToTemp(t)
	require.NoError(t, os.MkdirAll("./tmp", 0755))

	r := &Regius{}
	r.config.uploads.allowedTypes = []string{"image/png"}

	req := newMultipartUploadRequest(t, "file", "test.png", pngSignature)
	name, err := r.getFileToUpload(req, "file")

	require.NoError(t, err)
	assert.Equal(t, "./tmp/test.png", name)

	_, statErr := os.Stat("./tmp/test.png")
	assert.NoError(t, statErr, "uploaded file should exist in ./tmp")
}

func TestGetFileToUpload_InvalidMimeType(t *testing.T) {
	chdirToTemp(t)
	require.NoError(t, os.MkdirAll("./tmp", 0755))

	r := &Regius{}
	r.config.uploads.allowedTypes = []string{"image/jpeg"} // png not allowed

	req := newMultipartUploadRequest(t, "file", "test.png", pngSignature)
	_, err := r.getFileToUpload(req, "file")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mime type")
}

func TestGetFileToUpload_MissingField(t *testing.T) {
	chdirToTemp(t)
	require.NoError(t, os.MkdirAll("./tmp", 0755))

	r := &Regius{}
	r.config.uploads.allowedTypes = []string{"image/png"}

	// A multipart request without the expected field.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	_, err := r.getFileToUpload(req, "file")
	require.Error(t, err)
}

func TestUploadFile_WithFakeFS(t *testing.T) {
	chdirToTemp(t)
	require.NoError(t, os.MkdirAll("./tmp", 0755))

	r := &Regius{ErrorLog: log.New(io.Discard, "", 0)}
	r.config.uploads.allowedTypes = []string{"image/png"}

	fs := &fakeFS{}
	req := newMultipartUploadRequest(t, "file", "test.png", pngSignature)

	err := r.UploadFile(req, "uploads", "file", fs)
	require.NoError(t, err)

	require.Len(t, fs.putCalls, 1)
	assert.Equal(t, "./tmp/test.png", fs.putCalls[0].fileName)
	assert.Equal(t, "uploads", fs.putCalls[0].folder)

	_, statErr := os.Stat("./tmp/test.png")
	assert.Error(t, statErr, "temp file should be removed after upload to FS")
}

func TestUploadFile_LocalRename(t *testing.T) {
	chdirToTemp(t)
	require.NoError(t, os.MkdirAll("./tmp", 0755))

	dest := "./destination"
	require.NoError(t, os.MkdirAll(dest, 0755))

	r := &Regius{ErrorLog: log.New(io.Discard, "", 0)}
	r.config.uploads.allowedTypes = []string{"image/png"}

	req := newMultipartUploadRequest(t, "file", "test.png", pngSignature)

	err := r.UploadFile(req, dest, "file", nil)
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(dest, "test.png"))
	assert.NoError(t, statErr, "file should be moved to the destination")
}
