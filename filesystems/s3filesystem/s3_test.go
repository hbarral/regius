package s3filesystem

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeService struct {
	listOutput *s3.ListObjectsOutput
	listErr    error
	deleteErr  error
	listed     []*s3.ListObjectsInput
	deleted    []*s3.DeleteObjectsInput
}

func (f *fakeService) ListObjects(input *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
	f.listed = append(f.listed, input)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listOutput, nil
}

func (f *fakeService) DeleteObjects(input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
	f.deleted = append(f.deleted, input)
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &s3.DeleteObjectsOutput{}, nil
}

type downloadCall struct {
	bucket, key string
}

type fakeDownloader struct {
	downloads []downloadCall
	err       error
	content   []byte
}

func (f *fakeDownloader) Download(w io.WriterAt, input *s3.GetObjectInput, options ...func(*s3manager.Downloader)) (int64, error) {
	f.downloads = append(f.downloads, downloadCall{
		bucket: aws.StringValue(input.Bucket),
		key:    aws.StringValue(input.Key),
	})
	if f.err != nil {
		return 0, f.err
	}
	n, err := w.WriteAt(f.content, 0)
	return int64(n), err
}

type uploadCall struct {
	bucket, key string
	content     []byte
}

type fakeUploader struct {
	uploads []uploadCall
	err     error
}

func (f *fakeUploader) Upload(input *s3manager.UploadInput, options ...func(*s3manager.Uploader)) (*s3manager.UploadOutput, error) {
	content, _ := io.ReadAll(input.Body)
	f.uploads = append(f.uploads, uploadCall{
		bucket:  aws.StringValue(input.Bucket),
		key:     aws.StringValue(input.Key),
		content: content,
	})
	if f.err != nil {
		return nil, f.err
	}
	return &s3manager.UploadOutput{Location: aws.StringValue(input.Key)}, nil
}

func newTestS3(service s3Service, downloader s3Downloader, uploader s3Uploader) *S3 {
	s := &S3{
		Bucket: "test-bucket",
	}
	if service != nil {
		s.serviceFactory = func() (s3Service, error) { return service, nil }
	}
	if downloader != nil {
		s.downloaderFactory = func() (s3Downloader, error) { return downloader, nil }
	}
	if uploader != nil {
		s.uploaderFactory = func() (s3Uploader, error) { return uploader, nil }
	}
	return s
}

func TestS3_List(t *testing.T) {
	fake := &fakeService{
		listOutput: &s3.ListObjectsOutput{
			Contents: []*s3.Object{
				{
					Key:          aws.String("file.txt"),
					Size:         aws.Int64(1024),
					ETag:         aws.String("etag"),
					LastModified: aws.Time(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
				},
			},
		},
	}
	s := newTestS3(fake, nil, nil)

	listing, err := s.List("/")

	require.NoError(t, err)
	require.Len(t, listing, 1)
	assert.Equal(t, "file.txt", listing[0].Key)
	assert.Equal(t, "etag", listing[0].Etag)
	assert.InDelta(t, 0.0009765625, listing[0].Size, 0.0000001)

	require.Len(t, fake.listed, 1)
	assert.Equal(t, "test-bucket", aws.StringValue(fake.listed[0].Bucket))
	assert.Equal(t, "", aws.StringValue(fake.listed[0].Prefix))
}

func TestS3_List_WithPrefix(t *testing.T) {
	fake := &fakeService{listOutput: &s3.ListObjectsOutput{}}
	s := newTestS3(fake, nil, nil)

	_, err := s.List("prefix/")

	require.NoError(t, err)
	require.Len(t, fake.listed, 1)
	assert.Equal(t, "prefix/", aws.StringValue(fake.listed[0].Prefix))
}

func TestS3_List_Error(t *testing.T) {
	fake := &fakeService{
		listErr: awserr.New(s3.ErrCodeNoSuchBucket, "no such bucket", nil),
	}
	s := newTestS3(fake, nil, nil)

	_, err := s.List("/")

	assert.Error(t, err)
}

func TestS3_List_CredentialsError(t *testing.T) {
	s := &S3{
		Bucket:         "test-bucket",
		serviceFactory: func() (s3Service, error) { return nil, errors.New("dial failed") },
	}

	_, err := s.List("/")
	assert.EqualError(t, err, "dial failed")
}

func TestS3_Get(t *testing.T) {
	fake := &fakeDownloader{content: []byte("downloaded content")}
	s := newTestS3(nil, fake, nil)

	destDir := t.TempDir()
	err := s.Get(destDir, "remote.txt")

	require.NoError(t, err)
	require.Len(t, fake.downloads, 1)
	assert.Equal(t, "test-bucket", fake.downloads[0].bucket)
	assert.Equal(t, "remote.txt", fake.downloads[0].key)

	content, err := os.ReadFile(filepath.Join(destDir, "remote.txt"))
	require.NoError(t, err)
	assert.Equal(t, "downloaded content", string(content))
}

func TestS3_Get_Error(t *testing.T) {
	fake := &fakeDownloader{err: errors.New("download failed")}
	s := newTestS3(nil, fake, nil)

	err := s.Get(t.TempDir(), "remote.txt")

	assert.EqualError(t, err, "download failed")
}

func TestS3_Get_CredentialsError(t *testing.T) {
	s := &S3{
		Bucket:            "test-bucket",
		downloaderFactory: func() (s3Downloader, error) { return nil, errors.New("dial failed") },
	}

	err := s.Get(t.TempDir(), "remote.txt")
	assert.EqualError(t, err, "dial failed")
}

func TestS3_Put(t *testing.T) {
	fake := &fakeUploader{}
	s := newTestS3(nil, nil, fake)

	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("file content"), 0644))

	err := s.Put(localFile, "/")

	require.NoError(t, err)
	require.Len(t, fake.uploads, 1)
	assert.Equal(t, "test-bucket", fake.uploads[0].bucket)
	assert.Equal(t, "/upload.txt", fake.uploads[0].key)
	assert.Equal(t, "file content", string(fake.uploads[0].content))
}

func TestS3_Put_Error(t *testing.T) {
	fake := &fakeUploader{err: errors.New("upload failed")}
	s := newTestS3(nil, nil, fake)

	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "upload.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("content"), 0644))

	err := s.Put(localFile, "/")

	assert.EqualError(t, err, "upload failed")
}

func TestS3_Put_CredentialsError(t *testing.T) {
	s := &S3{
		Bucket:          "test-bucket",
		uploaderFactory: func() (s3Uploader, error) { return nil, errors.New("dial failed") },
	}

	err := s.Put("/nonexistent/file.txt", "/")

	assert.EqualError(t, err, "dial failed")
}

func TestS3_Delete(t *testing.T) {
	fake := &fakeService{}
	s := newTestS3(fake, nil, nil)

	ok := s.Delete([]string{"file1.txt", "file2.txt"})

	assert.True(t, ok)
	require.Len(t, fake.deleted, 2)
	assert.Equal(t, "file1.txt", aws.StringValue(fake.deleted[0].Delete.Objects[0].Key))
	assert.Equal(t, "file2.txt", aws.StringValue(fake.deleted[1].Delete.Objects[0].Key))
}

func TestS3_Delete_Error(t *testing.T) {
	fake := &fakeService{deleteErr: errors.New("delete failed")}
	s := newTestS3(fake, nil, nil)

	ok := s.Delete([]string{"file1.txt"})

	assert.False(t, ok)
}

func TestS3_Delete_CredentialsError(t *testing.T) {
	s := &S3{
		Bucket:         "test-bucket",
		serviceFactory: func() (s3Service, error) { return nil, errors.New("dial failed") },
	}

	ok := s.Delete([]string{"file1.txt"})

	assert.False(t, ok)
}
