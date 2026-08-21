package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"path"
	"path/filepath"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type OSS struct {
	ID       string //
	Key      string //
	Endpoint string //
	Bucket   string //
	Folder   string //
	Client   *oss.Client
}

func initOSSClient(data *OSS) (*oss.Client, error) {
	client, err := oss.New(data.Endpoint, data.ID, data.Key)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return client, nil
}

func (c *Client) uploadFile(file io.Reader, path string) error {
	bucket, err := c.Client.Bucket(c.Bucket)
	if err != nil {
		return err
	}

	return bucket.PutObject(path, file)
}

func (c *Client) uploadProduct(product *Product) error {
	if product == nil {
		return errors.New("product is nil")
	}
	if product.Data == nil {
		return errors.New("product data is nil")
	}

	prefix := path.Join(c.Folder, product.Version+"-"+product.UUID, product.System)
	productKey := path.Join(prefix, product.FileName)

	if err := c.uploadFile(product.Data, productKey); err != nil {
		return err
	}
	product.URL = productKey

	for i := range product.Files {
		file := &product.Files[i]
		fileKey := path.Join(prefix, file.Path)
		fileReader := bytes.NewReader(file.Data)
		if err := c.uploadFile(fileReader, fileKey); err != nil {
			return err
		}
		file.URL = fileKey
	}
	return nil
}

func (c *Client) DownloadFile(path string) ([]byte, error) {
	bucket, err := c.Client.Bucket(c.Bucket)
	if err != nil {
		return nil, err
	}
	body, err := bucket.GetObject(path)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return io.ReadAll(body)
}

func (c *Client) DownloadPatch(files []File) ([]byte, error) {
	if c == nil || c.Client == nil {
		return nil, errors.New("OSS client is nil")
	}

	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)

	manifest, err := json.Marshal(files)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := writeTarEntry(tarWriter, "manifest.json", manifest); err != nil {
		return nil, err
	}
	archiveNames := map[string]struct{}{"manifest.json": {}}

	for _, file := range files {
		data, err := c.DownloadFile(file.URL)
		if err != nil {
			return nil, fmt.Errorf("download file %s: %w", file.Path, err)
		}

		name := path.Clean(filepath.ToSlash(file.Path))
		if filepath.IsAbs(file.Path) || name == "." || name == ".." ||
			strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
			return nil, fmt.Errorf("invalid file path: %s", file.Path)
		}
		if _, exists := archiveNames[name]; exists {
			return nil, fmt.Errorf("duplicate archive file name: %s", name)
		}
		archiveNames[name] = struct{}{}
		if uint64(len(data)) != file.Size {
			return nil, fmt.Errorf("size mismatch for %s: got %d, want %d", name, len(data), file.Size)
		}
		header := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(data)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("write tar header %s: %w", name, err)
		}
		if _, err := tarWriter.Write(data); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}
	return output.Bytes(), nil
}

func writeTarEntry(writer *tar.Writer, name string, data []byte) error {
	if err := writer.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(data)),
	}); err != nil {
		return fmt.Errorf("write tar header %s: %w", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write tar entry %s: %w", name, err)
	}
	return nil
}

func (c *Client) DownloadLatestSetup() (*Product, error) {
	latest, err := c.getLatestProduct()
	if err != nil {
		return nil, err
	}
	body, err := c.DownloadFile(latest.URL)
	if err != nil {
		return nil, err
	}
	latest.Bytes = body
	return &latest, nil
}
