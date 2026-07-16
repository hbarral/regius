package miniofilesystem

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/hbarral/regius/filesystems"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type minioClient interface {
	FPutObject(ctx context.Context, bucket, objectName, filePath string, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
	RemoveObject(ctx context.Context, bucket, objectName string, opts minio.RemoveObjectOptions) error
	FGetObject(ctx context.Context, bucket, objectName, filePath string, opts minio.GetObjectOptions) error
}

type Minio struct {
	Endpoint string
	Key      string
	Secret   string
	UseSSL   bool
	Region   string
	Bucket   string
	// clientFactory, when non-nil, overrides the default minio.New dial.
	// It is used by tests to inject a fake minio client.
	clientFactory func() (minioClient, error)
}

func (m *Minio) getCredentials() (minioClient, error) {
	if m.clientFactory != nil {
		return m.clientFactory()
	}

	client, err := minio.New(m.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(m.Key, m.Secret, ""),
		Secure: m.UseSSL,
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (m *Minio) Put(fileName, folder string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := m.getCredentials()
	if err != nil {
		return err
	}

	objectName := path.Base(fileName)

	uploadInfo, err := client.FPutObject(
		ctx,
		m.Bucket,
		path.Join(folder, objectName),
		fileName,
		minio.PutObjectOptions{},
	)
	if err != nil {
		log.Println("Failed with FPutObject")
		log.Println(err)
		log.Println("UploadInfo:", uploadInfo)
		return err
	}

	return nil
}

func (m *Minio) List(prefix string) ([]filesystems.Listing, error) {
	var listing []filesystems.Listing

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := m.getCredentials()
	if err != nil {
		return listing, err
	}

	objectCh := client.ListObjects(ctx, m.Bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			fmt.Println(object.Err)
			return listing, object.Err
		}

		if !strings.HasPrefix(object.Key, ".") {
			b := float64(object.Size)
			kb := b / 1024
			mb := kb / 1024
			item := filesystems.Listing{
				Etag:         object.ETag,
				LastModified: object.LastModified,
				Key:          object.Key,
				Size:         mb,
			}
			listing = append(listing, item)
		}
	}

	return listing, nil
}

func (m *Minio) Delete(itemsToDelete []string) bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := m.getCredentials()
	if err != nil {
		return false
	}

	opts := minio.RemoveObjectOptions{
		GovernanceBypass: true,
	}

	for _, item := range itemsToDelete {
		err := client.RemoveObject(ctx, m.Bucket, item, opts)
		if err != nil {
			fmt.Println(err)
			return false
		}
	}
	return true
}

func (m *Minio) Get(destination string, items ...string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := m.getCredentials()
	if err != nil {
		return err
	}

	for _, item := range items {
		err := client.FGetObject(
			ctx,
			m.Bucket,
			item,
			fmt.Sprintf("%s/%s", destination, path.Base(item)),
			minio.GetObjectOptions{},
		)
		if err != nil {
			fmt.Println(err)
			return err
		}
	}

	return nil
}
