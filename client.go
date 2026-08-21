package main

import (
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
	product, err := AnalyzeInnoSetupEXE(setup)
	if err != nil {
		return err
	}

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

func (c *Client) Compare(files []File) ([]File, error) {
	latest, err := c.getLatestProduct()
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
