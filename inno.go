package simpleupdater

import (
	"errors"
	"path/filepath"

	"github.com/Peiratooo/innoextract-go"
)

func AnalyzeInnoSetupEXE(setup SetupReader) (*Product, error) {
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
		AppID:   info.AppID,
	}

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
