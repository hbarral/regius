package miniofilesystem

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"

	minio "github.com/minio/minio-go/v7"

	credentials "github.com/minio/minio-go/v7/pkg/credentials"
)

type Minio struct {
	Endpoint string
	Key      string
	Secret   string
	UseSSL   bool
	Region   string
	Bucket   string
}

func (m *Minio) getCredentials() *minio.Client {
	client, err := minio.New(m.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(m.Key, m.Secret, ""),
		Secure: m.UseSSL,
	})
	if err != nil {
		log.Println(err)
	}

	return client
}

func (m *Minio) Put(fileName, folder string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	objectName := path.Base(fileName)
	client := m.getCredentials()

	uploadInfo, err := client.FPutObject(
		ctx,
		m.Bucket,
		fmt.Sprintf("%s/%s", folder, objectName),
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

	client := m.getCredentials()

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
				Etag:         object.Etag,
				LastModified: object.LastModified,
				Key:          object.Key,
				Size:         mb,
			}
			lising = append(listing, item)
		}
	}

	return listing, nil
}
