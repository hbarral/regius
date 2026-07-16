package miniofilesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type uploadCall struct {
	bucket, objectName, filePath string
}

type getCall struct {
	bucket, objectName, filePath string
}

type fakeMinioClient struct {
	uploadInfo minio.UploadInfo
	uploadErr  error
	uploaded   []uploadCall

	listObjs <-chan minio.ObjectInfo

	removeErr error
	removed   []string

	getErr error
	got    []getCall
}

func (f *fakeMinioClient) FPutObject(ctx context.Context, bucket, objectName, filePath string, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	f.uploaded = append(f.uploaded, uploadCall{bucket, objectName, filePath})
	return f.uploadInfo, f.uploadErr
}

func (f *fakeMinioClient) ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	return f.listObjs
}

func (f *fakeMinioClient) RemoveObject(ctx context.Context, bucket, objectName string, opts minio.RemoveObjectOptions) error {
	f.removed = append(f.removed, objectName)
	return f.removeErr
}

func (f *fakeMinioClient) FGetObject(ctx context.Context, bucket, objectName, filePath string, opts minio.GetObjectOptions) error {
	f.got = append(f.got, getCall{bucket, objectName, filePath})
	if f.getErr != nil {
		return f.getErr
	}
	return os.WriteFile(filePath, []byte("downloaded"), 0644)
}

func objectInfoChan(infos ...minio.ObjectInfo) <-chan minio.ObjectInfo {
	ch := make(chan minio.ObjectInfo)
	go func() {
		defer close(ch)
		for _, info := range infos {
			ch <- info
		}
	}()
	return ch
}

func newTestMinio(client minioClient) *Minio {
	return &Minio{
		Bucket:        "test-bucket",
		clientFactory: func() (minioClient, error) { return client, nil },
	}
}

func TestMinio_Put(t *testing.T) {
	fake := &fakeMinioClient{uploadInfo: minio.UploadInfo{Bucket: "test-bucket", Key: "upload.txt", Size: 7}}
	m := newTestMinio(fake)

	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("content"), 0644))

	err := m.Put(localFile, "/")

	require.NoError(t, err)
	require.Len(t, fake.uploaded, 1)
	assert.Equal(t, "test-bucket", fake.uploaded[0].bucket)
	assert.Equal(t, "/upload.txt", fake.uploaded[0].objectName)
	assert.Equal(t, localFile, fake.uploaded[0].filePath)
}

func TestMinio_Put_Error(t *testing.T) {
	fake := &fakeMinioClient{uploadErr: errors.New("upload failed")}
	m := newTestMinio(fake)

	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("content"), 0644))

	err := m.Put(localFile, "/")

	assert.EqualError(t, err, "upload failed")
}

func TestMinio_List(t *testing.T) {
	fake := &fakeMinioClient{
		listObjs: objectInfoChan(minio.ObjectInfo{
			Key:          "file.txt",
			Size:         1024,
			ETag:         "etag",
			LastModified: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}),
	}
	m := newTestMinio(fake)

	listing, err := m.List("/")

	require.NoError(t, err)
	require.Len(t, listing, 1)
	assert.Equal(t, "file.txt", listing[0].Key)
	assert.InDelta(t, 0.0009765625, listing[0].Size, 0.0000001) // 1024 bytes -> MB
	assert.Equal(t, "etag", listing[0].Etag)
}

func TestMinio_List_ChannelError(t *testing.T) {
	ch := make(chan minio.ObjectInfo, 1)
	ch <- minio.ObjectInfo{Err: errors.New("list failed")}
	close(ch)

	fake := &fakeMinioClient{listObjs: ch}
	m := newTestMinio(fake)

	_, err := m.List("/")

	assert.EqualError(t, err, "list failed")
}

func TestMinio_Delete(t *testing.T) {
	fake := &fakeMinioClient{}
	m := newTestMinio(fake)

	ok := m.Delete([]string{"file1.txt", "file2.txt"})

	assert.True(t, ok)
	assert.Equal(t, []string{"file1.txt", "file2.txt"}, fake.removed)
}

func TestMinio_Delete_Error(t *testing.T) {
	fake := &fakeMinioClient{removeErr: errors.New("remove failed")}
	m := newTestMinio(fake)

	ok := m.Delete([]string{"file1.txt"})

	assert.False(t, ok)
	assert.Equal(t, []string{"file1.txt"}, fake.removed)
}

func TestMinio_Delete_CredentialsError(t *testing.T) {
	m := &Minio{
		Bucket:        "test-bucket",
		clientFactory: func() (minioClient, error) { return nil, errors.New("dial failed") },
	}

	ok := m.Delete([]string{"file1.txt"})

	assert.False(t, ok)
}

func TestMinio_Get(t *testing.T) {
	fake := &fakeMinioClient{}
	m := newTestMinio(fake)

	destDir := t.TempDir()
	err := m.Get(destDir, "remote.txt")

	require.NoError(t, err)
	require.Len(t, fake.got, 1)
	assert.Equal(t, "test-bucket", fake.got[0].bucket)
	assert.Equal(t, "remote.txt", fake.got[0].objectName)
	assert.Equal(t, filepath.Join(destDir, "remote.txt"), fake.got[0].filePath)

	content, err := os.ReadFile(fake.got[0].filePath)
	require.NoError(t, err)
	assert.Equal(t, "downloaded", string(content))
}

func TestMinio_Get_Error(t *testing.T) {
	fake := &fakeMinioClient{getErr: errors.New("download failed")}
	m := newTestMinio(fake)

	destDir := t.TempDir()
	err := m.Get(destDir, "remote.txt")

	assert.EqualError(t, err, "download failed")
}

func TestMinio_Get_CredentialsError(t *testing.T) {
	m := &Minio{
		Bucket:        "test-bucket",
		clientFactory: func() (minioClient, error) { return nil, errors.New("dial failed") },
	}

	err := m.Get(t.TempDir(), "remote.txt")

	assert.EqualError(t, err, "dial failed")
}

func TestMinio_Put_CredentialsError(t *testing.T) {
	m := &Minio{
		Bucket:        "test-bucket",
		clientFactory: func() (minioClient, error) { return nil, errors.New("dial failed") },
	}

	err := m.Put("/nonexistent/file.txt", "/")

	assert.EqualError(t, err, "dial failed")
}

func TestMinio_List_CredentialsError(t *testing.T) {
	m := &Minio{
		Bucket:        "test-bucket",
		clientFactory: func() (minioClient, error) { return nil, errors.New("dial failed") },
	}

	_, err := m.List("/")

	assert.EqualError(t, err, "dial failed")
}
