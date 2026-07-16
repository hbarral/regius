package sftpfilesystem

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"

	"github.com/hbarral/regius/filesystems"
	"github.com/pkg/sftp"

	"golang.org/x/crypto/ssh"
)

type SFTP struct {
	Host string
	Port string
	User string
	Pass string
	// clientFactory, when non-nil, overrides the default SSH dial.
	// It is used by tests to inject an in-process SFTP client.
	clientFactory func() (*sftp.Client, error)
}

func (s *SFTP) getCredentials() (*sftp.Client, error) {
	if s.clientFactory != nil {
		client, err := s.clientFactory()
		if err != nil {
			return nil, err
		}

		cwd, err := client.Getwd()
		if err != nil {
			return nil, err
		}
		log.Println("Current working directory:", cwd)

		return client, nil
	}

	addr := fmt.Sprintf("%s:%s", s.Host, s.Port)
	config := &ssh.ClientConfig{
		User: s.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.Pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, err
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		return nil, err
	}

	cwd, err := client.Getwd()
	if err != nil {
		return nil, err
	}
	log.Println("Current working directory:", cwd)

	return client, nil
}

func normalizePath(p string) string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		p = "."
	}
	return p
}

func (s *SFTP) Put(fileName, folder string) error {
	client, err := s.getCredentials()
	if err != nil {
		return err
	}
	defer client.Close()

	sourceFile, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := client.Create(path.Join(normalizePath(folder), path.Base(fileName)))
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return err
	}

	return nil
}

func (s *SFTP) List(prefix string) ([]filesystems.Listing, error) {
	var listing []filesystems.Listing
	client, err := s.getCredentials()
	if err != nil {
		return listing, err
	}
	defer client.Close()

	files, err2 := client.ReadDir(normalizePath(prefix))
	if err2 != nil {
		return listing, err2
	}

	for _, x := range files {
		var item filesystems.Listing

		if !strings.HasPrefix(x.Name(), ".") {
			b := float64(x.Size())
			kb := b / 1024
			mb := kb / 1024
			item.Key = x.Name()
			item.Size = mb
			item.LastModified = x.ModTime()
			item.IsDir = x.IsDir()
			listing = append(listing, item)
		}
	}

	return listing, nil
}

func (s *SFTP) Delete(itemsToDelete []string) bool {
	client, err := s.getCredentials()
	if err != nil {
		return false
	}
	defer client.Close()

	for _, x := range itemsToDelete {
		deleteErr := client.Remove(normalizePath(x))
		if deleteErr != nil {
			return false
		}
	}

	return true
}

func (s *SFTP) Get(destination string, items ...string) error {
	client, err := s.getCredentials()
	if err != nil {
		return err
	}
	defer client.Close()

	for _, x := range items {
		err := func() error {
			sourceFile, err2 := client.Open(normalizePath(x))
			if err2 != nil {
				return err2
			}
			defer sourceFile.Close()

			destinationFile, err := os.Create(fmt.Sprintf("%s/%s", destination, path.Base(x)))
			if err != nil {
				return err
			}
			defer destinationFile.Close()

			bytes, err := io.Copy(destinationFile, sourceFile)
			if err != nil {
				return err
			}
			fmt.Println("Copied", bytes, "bytes")

			err = destinationFile.Sync()
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
