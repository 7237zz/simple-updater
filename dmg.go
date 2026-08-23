package simpleupdater

import (
	"errors"
	"os"
	"path/filepath"
)

func AnalyzeSetupDMG(setup *os.File) (*Product, error) {
	if setup == nil {
		return nil, errors.New("setup file is nil")
	}

	app, cleanup, err := ExtractDMGApp(setup)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	info, err := ReadInfoPlist(app)
	if err != nil {
		return nil, err
	}

	product := &Product{
		Product: info.AppName,
		Version: info.AppVersion,
		AppID:   info.AppID,
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

	product.FileName = filepath.Base(setup.Name())

	product.Files, err = ScanRoot(app)
	if err != nil {
		return nil, err
	}

	return product, nil
}
