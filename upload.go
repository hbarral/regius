package regius

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/gabriel-vasile/mimetype"
	"gitlab.com/hbarral/regius/filesystems"
)

func (r *Regius) UploadFile(
	res *http.Request,
	destination string,
	field string,
	fs filesystems.FS,
) error {
	fileName, err := r.getFileToUpload(res, field)
	if err != nil {
		r.ErrorLog.Println(err)
		return err
	}

	if fs != nil {
		err := fs.Put(fileName, destination)
		if err != nil {
			r.ErrorLog.Println(err)
			return err
		}
	}

	if fs == nil {
		err := os.Rename(fileName, fmt.Sprintf("%s/%s", destination, path.Base(fileName)))
		if err != nil {
			r.ErrorLog.Println(err)
			return err
		}
	}

	return nil
}

func (r *Regius) getFileToUpload(req *http.Request, fieldName string) (string, error) {
	_ = req.ParseMultipartForm(10 << 20)

	file, handler, err := req.FormFile(fieldName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	mimeType, err := mimetype.DetectReader(file)
	if err != nil {
		return "", err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return "", err
	}

	if !inSlice(r.config.uploads.allowedTypes, mimeType.String()) {
		return "", fmt.Errorf("invalid mime type: %s", mimeType.String())
	}

	destination, err := os.Create(fmt.Sprintf("./tmp/%s", handler.Filename))
	if err != nil {
		return "", err
	}
	defer destination.Close()

	_, err = io.Copy(destination, file)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("./tmp/%s", handler.Filename), nil
}

func inSlice(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}
