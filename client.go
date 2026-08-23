package main

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
		if !exists || oldFile.SHA256 != latestFile.SHA256 {
			result = append(result, latestFile)
		}
	}
	return result, nil
}
