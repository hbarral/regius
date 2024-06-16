package webdavfilesystem

import (
	"fmt"
	"os"
	"path"
	"strings"

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
	client := w.getCredentials()
	files, err := client.ReadDir(prefix)
	if err != nil {
		return listing, err
	}

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), ".") {
			b := float64(file.Size())
			kb := b / 1024
			mb := kb / 1024
			current := filesystems.Listing{
				LastModified: file.ModTime(),
				Key:          file.Name(),
				Size:         mb,
				IsDir:        file.IsDir(),
			}

			listing = append(listing, current)
		}
	}

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
	client := w.getCredentials()
	for _, item := range itemsToDelete {
		err := client.Remove(item)
		if err != nil {
			return false
		}
	}

	return true
}
