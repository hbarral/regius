package s3filesystem

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hbarral/regius/filesystems"
)

type s3Service interface {
	ListObjects(input *s3.ListObjectsInput) (*s3.ListObjectsOutput, error)
	DeleteObjects(input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error)
}

type s3Downloader interface {
	Download(w io.WriterAt, input *s3.GetObjectInput, options ...func(*s3manager.Downloader)) (int64, error)
}

type s3Uploader interface {
	Upload(input *s3manager.UploadInput, options ...func(*s3manager.Uploader)) (*s3manager.UploadOutput, error)
}

type S3 struct {
	Key      string
	Secret   string
	Region   string
	Endpoint string
	Bucket   string
	// serviceFactory, when non-nil, overrides the default S3 service creation.
	// Used by tests to inject a fake S3 service.
	serviceFactory func() (s3Service, error)
	// downloaderFactory, when non-nil, overrides the default S3 downloader creation.
	// Used by tests to inject a fake downloader.
	downloaderFactory func() (s3Downloader, error)
	// uploaderFactory, when non-nil, overrides the default S3 uploader creation.
	// Used by tests to inject a fake uploader.
	uploaderFactory func() (s3Uploader, error)
}

func (s *S3) getCredentials() *credentials.Credentials {
	client := credentials.NewStaticCredentials(s.Key, s.Secret, "")
	return client
}

func (s *S3) getService() (s3Service, error) {
	if s.serviceFactory != nil {
		return s.serviceFactory()
	}

	client := s.getCredentials()
	sess, err := session.NewSession(&aws.Config{
		Endpoint:    &s.Endpoint,
		Region:      &s.Region,
		Credentials: client,
	})
	if err != nil {
		return nil, err
	}

	return s3.New(sess), nil
}

func (s *S3) getDownloader() (s3Downloader, error) {
	if s.downloaderFactory != nil {
		return s.downloaderFactory()
	}

	client := s.getCredentials()
	sess, err := session.NewSession(&aws.Config{
		Endpoint:    &s.Endpoint,
		Region:      &s.Region,
		Credentials: client,
	})
	if err != nil {
		return nil, err
	}

	return s3manager.NewDownloader(sess), nil
}

func (s *S3) getUploader() (s3Uploader, error) {
	if s.uploaderFactory != nil {
		return s.uploaderFactory()
	}

	client := s.getCredentials()
	sess, err := session.NewSession(&aws.Config{
		Endpoint:    &s.Endpoint,
		Region:      &s.Region,
		Credentials: client,
	})
	if err != nil {
		return nil, err
	}

	return s3manager.NewUploader(sess), nil
}

func (s *S3) List(prefix string) ([]filesystems.Listing, error) {
	var listing []filesystems.Listing

	if prefix == "/" {
		prefix = ""
	}

	service, err := s.getService()
	if err != nil {
		return nil, err
	}

	input := &s3.ListObjectsInput{
		Bucket: aws.String(s.Bucket),
		Prefix: aws.String(prefix),
	}

	result, err := service.ListObjects(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				fmt.Println(s3.ErrCodeNoSuchBucket, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		return nil, err
	}

	for _, item := range result.Contents {
		b := float64(*item.Size)
		kb := b / 1024
		mb := kb / 1024
		item := filesystems.Listing{
			Etag:         *item.ETag,
			LastModified: *item.LastModified,
			Key:          *item.Key,
			Size:         mb,
		}
		listing = append(listing, item)
	}

	return listing, nil
}

func (s *S3) Get(destination string, items ...string) error {
	downloader, err := s.getDownloader()
	if err != nil {
		return err
	}

	for _, item := range items {
		err := func() error {
			file, err := os.Create(fmt.Sprintf("%s/%s", destination, item))
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = downloader.Download(file, &s3.GetObjectInput{
				Bucket: aws.String(s.Bucket),
				Key:    aws.String(item),
			})
			if err != nil {
				return err
			}

			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *S3) Put(fileName, folder string) error {
	uploader, err := s.getUploader()
	if err != nil {
		return err
	}

	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	size := fileInfo.Size()

	buffer := make([]byte, size)
	_, err = file.Read(buffer)
	if err != nil {
		return err
	}

	fileBytes := bytes.NewReader(buffer)
	fileType := http.DetectContentType(buffer)

	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(path.Join(folder, path.Base(fileName))),
		Body:   fileBytes,
		// ACL:         aws.String("public-read"),
		ContentType: aws.String(fileType),
		Metadata: map[string]*string{
			"Key": aws.String("MetadataValue"),
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *S3) Delete(itemsToDelete []string) bool {
	service, err := s.getService()
	if err != nil {
		return false
	}

	for _, item := range itemsToDelete {
		input := &s3.DeleteObjectsInput{
			Bucket: aws.String(s.Bucket),
			Delete: &s3.Delete{
				Objects: []*s3.ObjectIdentifier{
					{
						Key: aws.String(item),
					},
				},
				Quiet: aws.Bool(false),
			},
		}

		_, err := service.DeleteObjects(input)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case s3.ErrCodeNoSuchBucket:
					fmt.Println(s3.ErrCodeNoSuchBucket, aerr.Error())
				default:
					fmt.Println(aerr.Error())
				}
			} else {
				fmt.Println(err.Error())
			}
			return false
		}
	}

	return true
}
