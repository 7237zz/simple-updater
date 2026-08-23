package simpleupdater

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Peiratooo/innoextract-go"
)

func AnalyzeInnoSetupEXE(setup *os.File) (*Product, error) {
	if setup == nil {
		return nil, errors.New("setup file is nil")
	}

	archive, err := innoextract.Open(setup)
	if err != nil {
		return nil, err
	}
	info := archive.Info()
	product := &Product{
		Product: info.AppName,
		Version: info.AppVersion,
		Data:    setup,
	}

	product.SHA256, err = GenerateSHA256(setup)
	if err != nil {
		return nil, err
	}

	product.Size, err = GenerateSize(setup)
	if err != nil {
		return nil, err
	}

	product.AppID = info.AppID

	product.FileName = filepath.Base(setup.Name())

	files, err := archive.Extract()
	if err != nil {
		return nil, err
	}
	product.Files = make([]File, 0, len(files))
	for _, file := range files {
		product.Files = append(product.Files, File{
			Path:   filepath.ToSlash(file.Path),
			SHA256: file.SHA256,
			Size:   file.Size,
			Data:   file.Data,
		})
	}
	return product, nil
}
