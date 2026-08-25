package simpleupdater

import "errors"

func AnalyzeSetupDMG(setup SetupReader) (*Product, error) {
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
	}

	product.Files, err = ScanRoot(app)
	if err != nil {
		return nil, err
	}

	return product, nil
}
