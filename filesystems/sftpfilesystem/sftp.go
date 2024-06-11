package sftpfilesystem

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"

	"github.com/pkg/sftp"
	"gitlab.com/hbarral/regius/filesystems"

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

func (s *SFTP) List(prefix string) ([]filesystem.Listing, error) {
	var listing []fileSystems.Listing

	return listing, nil
}

func (s *SFTP) Delete(itemsToDelete []string) bool {
	return true
}

func (s *SFTP) Get(destination string, items ...string) error {
	return nil
}
