package regius

import (
	"crypto/rand"
	"os"
)

const (
	randomString = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_+"
)

func (r *Regius) RandomString(n int) string {
	a, b := make([]rune, n), []rune(randomString)

	for i := range a {
		p, _ := rand.Prime(rand.Reader, len(b))
		x, y := p.Uint64(), uint64(len(b))
		a[i] = b[x%y]
	}

	return string(a)
}

func (c *Regius) CreateDirIfNotExist(path string) error {
	const mode = 0755

	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.Mkdir(path, mode)

		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Regius) CreateFileIfNotExists(path string) error {
	var _, err = os.Stat(path)
	if os.IsNotExist(err) {
		var file, err = os.Create(path)
		if err != nil {
			return err
		}

		defer func(file *os.File) {
			_ = file.Close()
		}(file)
	}

	return nil
}
