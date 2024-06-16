package webdavfilesystem

import (
	"fmt"
	"os"
	"path"

	"github.com/studio-b12/gowebdav"
	"gitlab.com/hbarral/regius/filesystems"
)

type WebDAV struct {
	Host   string
	Port   string
	User   string
	Pass   string
	UseSSL bool
}

func (w *WebDAV) getCredentials() *gowebdav.Client {
	scheme := "http"

	if w.UseSSL {
		scheme = "https"
	}

	addr := fmt.Sprintf("%s://%s:%s", scheme, w.Host, w.Port)
	client := gowebdav.NewClient(addr, w.User, w.Pass)

	return client
}

func (w *WebDAV) List(prefix string) ([]filesystems.Listing, error) {
	var listing []filesystems.Listing
	return listing, nil
}

func (w *WebDAV) Get(destination string, items ...string) error {
	return nil
}

func (w *WebDAV) Put(fileName, folder string) error {
	client := w.getCredentials()

	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	err = client.WriteStream(fmt.Sprintf("%s/%s", folder, path.Base(fileName)), file, 0664)
	if err != nil {
		return err
	}

	return nil
}

func (w *WebDAV) Delete(itemsToDelete []string) bool {
	return true
}
