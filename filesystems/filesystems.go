package filesystems

import "time"

type FS interface {
	Put(fileName, folder string) error
	Get(destination string, items ...string) error
	List(prefix string) ([]Listen, error)
	Delete(itemsToDelete []string) bool
}

type Listen struct {
	Etag         string
	LastModified time.Time
	Key          string
	Size         float64
	IsDir        bool
}
