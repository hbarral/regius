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
}

func (s *SFTP) getCredentials() (*sftp.Client, error) {
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

func (s *SFTP) Put(fileName, folder string) error {
	client, err := s.getCredentials()
	if err != nil {
		return err
	}
	defer client.Close()

	source_file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer source_file.Close()

	destination_file, err := client.Create(fmt.Sprintf("%s/%s", folder, path.Base(fileName)))
	if err != nil {
		return err
	}
	defer destination_file.Close()

	if _, err := io.Copy(destination_file, source_file); err != nil {
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

	files, err2 := client.ReadDir(prefix)
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
		deleteErr := client.Remove(x)
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
			source_file, err2 := client.Open(x)
			if err2 != nil {
				return err
			}
			defer source_file.Close()

			destination_file, err := os.Create(fmt.Sprintf("%s/%s", destination, path.Base(x)))
			if err != nil {
				return err
			}
			defer destination_file.Close()

			bytes, err := io.Copy(destination_file, source_file)
			if err != nil {
				return err
			}
			fmt.Println("Copied", bytes, "bytes")

			err = destination_file.Sync()
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
