package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"strings"

	"github.com/deploymenttheory/go-apfs-v2/pkg/apfs"
	"github.com/deploymenttheory/go-apfs-v2/pkg/disk"
	"github.com/deploymenttheory/go-apfs-v2/pkg/hfsplus"
	"howett.net/plist"
)

type AppInfo struct {
	AppID      string `plist:"CFBundleIdentifier"`
	AppVersion string `plist:"CFBundleVersion"`
	AppName    string `plist:"CFBundleName"`
}

func ExtractDMGApp(setup *os.File) (fs.FS, func(), error) {
	if setup == nil {
		return nil, func() {}, errors.New("setup file is nil")
	}

	reader, offset, closer, err := disk.OpenWithOffset(setup.Name())
	if err != nil {
		return nil, func() {}, err
	}

	if offset != 0 {
		reader = io.NewSectionReader(reader, offset, math.MaxInt64-offset)
	}

	cleanup := func() {
		closer.Close()
	}

	var volume fs.FS
	var magic [4]byte
	if _, err := reader.ReadAt(magic[:], 32); err != nil {
		cleanup()
		return nil, func() {}, err
	}

	if string(magic[:]) == "NXSB" {
		container, err := apfs.Open(reader, nil)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		oldCleanup := cleanup
		cleanup = func() {
			container.Close()
			oldCleanup()
		}

		volume, err = container.Volume(0)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
	} else {
		var signature [2]byte
		if _, err := reader.ReadAt(signature[:], 1024); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		if string(signature[:]) != "H+" && string(signature[:]) != "HX" {
			cleanup()
			return nil, func() {}, errors.New("unsupported DMG filesystem")
		}

		volume, err = hfsplus.New(reader)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
	}

	app, err := findSingleApp(volume)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}

	return app, cleanup, nil
}

func findSingleApp(root fs.FS) (fs.FS, error) {
	var apps []string

	err := fs.WalkDir(root, ".", func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() || !strings.EqualFold(path.Ext(entry.Name()), ".app") {
			return nil
		}

		if info, err := fs.Stat(root, path.Join(filePath, "Contents", "Info.plist")); err == nil && !info.IsDir() {
			apps = append(apps, filePath)
		}

		return fs.SkipDir
	})
	if err != nil {
		return nil, err
	}

	if len(apps) != 1 {
		return nil, fmt.Errorf("DMG must contain exactly one valid .app, found %d", len(apps))
	}

	return fs.Sub(root, apps[0])
}

func ReadInfoPlist(app fs.FS) (AppInfo, error) {
	var info AppInfo

	data, err := fs.ReadFile(app, "Contents/Info.plist")
	if err != nil {
		return info, err
	}

	if _, err := plist.Unmarshal(data, &info); err != nil {
		return info, err
	}

	if info.AppID == "" {
		return info, errors.New("CFBundleIdentifier is empty")
	}
	if info.AppVersion == "" {
		return info, errors.New("CFBundleVersion is empty")
	}
	if info.AppName == "" {
		return info, errors.New("CFBundleName is empty")
	}

	return info, nil
}

func ScanRoot(root fs.FS) ([]File, error) {
	var files []File

	err := fs.WalkDir(root, ".", func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		data, err := fs.ReadFile(root, filePath)
		if err != nil {
			return err
		}

		hash := sha256.Sum256(data)
		files = append(files, File{
			Path:   filePath,
			Size:   uint64(info.Size()),
			SHA256: hex.EncodeToString(hash[:]),
			Mode:   uint32(info.Mode().Perm()),
			Data:   data,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}
