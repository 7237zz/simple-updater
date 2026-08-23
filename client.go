package simpleupdater

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
)

type Client struct {
	OSS
	DB
}

func New(client *Client) *Client {
	var err error
	client.OSS.Client, err = initOSSClient(&client.OSS)
	if err != nil {
		log.Fatal(err)
	}
	client.DB.Engine, err = initEngine(&client.DB)
	if err != nil {
		log.Fatal(err)
	}
	return client
}

func (c *Client) Push(setup *os.File) error {
	if setup == nil {
		return errors.New("setup file is nil")
	}

	system, err := AnalyzeSystem(setup)
	if err != nil {
		return err
	}

	var product *Product
	switch system {
	case "windows":
		product, err = AnalyzeInnoSetupEXE(setup)
	case "darwin":
		product, err = AnalyzeSetupDMG(setup)
	default:
		return fmt.Errorf("unsupported system: %s", system)
	}
	if err != nil {
		return err
	}
	product.System = system

	uuidStr, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	product.UUID = uuidStr.String()
	if err := c.uploadProduct(product); err != nil {
		return err
	}
	return c.uploadProduct2DB(*product)
}

func (c *Client) Compare(system string, appID string, files []File) ([]File, error) {
	latest, err := c.getLatestProduct(system, appID)
	if err != nil {
		return nil, err
	}
	oldByPath := make(map[string]File, len(files))
	for _, file := range files {
		oldByPath[file.Path] = file
	}

	result := make([]File, 0, len(latest.Files))
	for _, latestFile := range latest.Files {
		oldFile, exists := oldByPath[latestFile.Path]
		if !exists || !sameFileState(oldFile, latestFile) {
			result = append(result, latestFile)
		}
	}
	return result, nil
}

func sameFileState(current File, latest File) bool {
	if current.fileType() != latest.fileType() {
		return false
	}

	if latest.fileType() == FileTypeSymlink {
		return current.LinkTarget == latest.LinkTarget
	}

	if current.SHA256 != latest.SHA256 {
		return false
	}
	if latest.Mode != 0 && current.Mode != latest.Mode {
		return false
	}
	return true
}
