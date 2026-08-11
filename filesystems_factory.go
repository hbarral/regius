package regius

import (
	"os"
	"strings"

	"github.com/hbarral/regius/filesystems"
	"github.com/hbarral/regius/filesystems/miniofilesystem"
	"github.com/hbarral/regius/filesystems/s3filesystem"
	"github.com/hbarral/regius/filesystems/sftpfilesystem"
	"github.com/hbarral/regius/filesystems/webdavfilesystem"
)

// initFileSystems creates filesystem clients from environment variables.
// Each filesystem is only initialized when its env vars are present.
func (r *Regius) initFileSystems() map[string]interface{} {
	fileSystems := make(map[string]interface{})

	if os.Getenv("MINIO_SECRET") != "" {
		useSSL := false
		if strings.ToLower(os.Getenv("MINIO_USESSL")) == "true" {
			useSSL = true
		}
		minio := &miniofilesystem.Minio{
			Endpoint: os.Getenv("MINIO_ENDPOINT"),
			Key:      os.Getenv("MINIO_KEY"),
			Secret:   os.Getenv("MINIO_SECRET"),
			UseSSL:   useSSL,
			Region:   os.Getenv("MINIO_REGION"),
			Bucket:   os.Getenv("MINIO_BUCKET"),
		}
		fileSystems["MINIO"] = minio
		r.Minio = minio
	}

	if os.Getenv("SFTP_HOST") != "" {
		sftp := &sftpfilesystem.SFTP{
			Host: os.Getenv("SFTP_HOST"),
			Port: os.Getenv("SFTP_PORT"),
			User: os.Getenv("SFTP_USER"),
			Pass: os.Getenv("SFTP_PASS"),
		}
		fileSystems["SFTP"] = sftp
		r.SFTP = sftp
	}

	if os.Getenv("WEBDAV_HOST") != "" {
		useSSL := false
		if strings.ToLower(os.Getenv("WEBDAV_USESSL")) == "true" {
			useSSL = true
		}
		webdav := &webdavfilesystem.WebDAV{
			Host:   os.Getenv("WEBDAV_HOST"),
			Port:   os.Getenv("WEBDAV_PORT"),
			User:   os.Getenv("WEBDAV_USER"),
			Pass:   os.Getenv("WEBDAV_PASS"),
			UseSSL: useSSL,
		}
		fileSystems["WebDAV"] = webdav
		r.WebDAV = webdav
	}

	if os.Getenv("S3_KEY") != "" {
		s3 := &s3filesystem.S3{
			Key:      os.Getenv("S3_KEY"),
			Secret:   os.Getenv("S3_SECRET"),
			Region:   os.Getenv("S3_REGION"),
			Bucket:   os.Getenv("S3_BUCKET"),
			Endpoint: os.Getenv("S3_ENDPOINT"),
		}
		fileSystems["S3"] = s3
		r.S3 = s3
	}

	return fileSystems
}

// Ensure the concrete pointer types satisfy the FS interface.
var _ filesystems.FS = (*s3filesystem.S3)(nil)
var _ filesystems.FS = (*sftpfilesystem.SFTP)(nil)
var _ filesystems.FS = (*webdavfilesystem.WebDAV)(nil)
var _ filesystems.FS = (*miniofilesystem.Minio)(nil)
