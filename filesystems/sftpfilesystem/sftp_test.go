package sftpfilesystem

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClientFactory returns a factory that creates an in-process SFTP client
// connected to a server rooted at the given directory.
func newTestClientFactory(tb testing.TB, root string) func() (*sftp.Client, error) {
	tb.Helper()
	return func() (*sftp.Client, error) {
		serverConn, clientConn := net.Pipe()

		server, err := sftp.NewServer(serverConn, sftp.WithServerWorkingDirectory(root))
		if err != nil {
			return nil, err
		}
		go func() { _ = server.Serve() }()

		client, err := sftp.NewClientPipe(clientConn, clientConn)
		if err != nil {
			return nil, err
		}
		return client, nil
	}
}

func TestSFTP_List(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello"), 0644))

	s := &SFTP{clientFactory: newTestClientFactory(t, root)}
	listing, err := s.List("/")

	require.NoError(t, err)
	require.Len(t, listing, 1)
	assert.Equal(t, "file.txt", listing[0].Key)
}

func TestSFTP_List_Empty(t *testing.T) {
	root := t.TempDir()

	s := &SFTP{clientFactory: newTestClientFactory(t, root)}
	listing, err := s.List("/")

	require.NoError(t, err)
	assert.Empty(t, listing)
}

func TestSFTP_List_ClientFactoryError(t *testing.T) {
	s := &SFTP{clientFactory: func() (*sftp.Client, error) {
		return nil, errors.New("dial failed")
	}}

	_, err := s.List("/")
	assert.EqualError(t, err, "dial failed")
}

func TestSFTP_Put(t *testing.T) {
	root := t.TempDir()

	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("upload payload"), 0644))

	s := &SFTP{clientFactory: newTestClientFactory(t, root)}
	err := s.Put(localFile, "/")

	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(root, "upload.txt"))
	require.NoError(t, err)
	assert.Equal(t, "upload payload", string(content))
}

func TestSFTP_Get(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "remote.txt"), []byte("remote content"), 0644))

	s := &SFTP{clientFactory: newTestClientFactory(t, root)}

	destDir := t.TempDir()
	err := s.Get(destDir, "/remote.txt")

	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(destDir, "remote.txt"))
	require.NoError(t, err)
	assert.Equal(t, "remote content", string(content))
}

func TestSFTP_Get_NotFound(t *testing.T) {
	root := t.TempDir()

	s := &SFTP{clientFactory: newTestClientFactory(t, root)}

	destDir := t.TempDir()
	err := s.Get(destDir, "/does-not-exist.txt")

	assert.Error(t, err)
}

func TestSFTP_Delete(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "delete-me.txt"), []byte("x"), 0644))

	s := &SFTP{clientFactory: newTestClientFactory(t, root)}
	ok := s.Delete([]string{"/delete-me.txt"})

	assert.True(t, ok)
	_, statErr := os.Stat(filepath.Join(root, "delete-me.txt"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestSFTP_Delete_NotFound(t *testing.T) {
	root := t.TempDir()

	s := &SFTP{clientFactory: newTestClientFactory(t, root)}
	ok := s.Delete([]string{"/does-not-exist.txt"})

	assert.False(t, ok)
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/", "."},
		{"/foo", "foo"},
		{"/foo/bar", "foo/bar"},
		{"foo", "foo"},
		{"", "."},
	}

	for _, e := range tests {
		assert.Equal(t, e.expected, normalizePath(e.input), "input: %q", e.input)
	}
}

func BenchmarkSFTP_Put(b *testing.B) {
	root := b.TempDir()
	localDir := b.TempDir()
	localFile := filepath.Join(localDir, "upload.txt")
	require.NoError(b, os.WriteFile(localFile, []byte("payload"), 0644))

	s := &SFTP{clientFactory: newTestClientFactory(b, root)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Put(localFile, "/")
	}
}

func BenchmarkSFTP_Get(b *testing.B) {
	root := b.TempDir()
	require.NoError(b, os.WriteFile(filepath.Join(root, "remote.txt"), []byte("payload"), 0644))

	s := &SFTP{clientFactory: newTestClientFactory(b, root)}
	destDir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Get(destDir, "/remote.txt")
	}
}
