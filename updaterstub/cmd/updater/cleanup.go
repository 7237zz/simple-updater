package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func temporaryHelperDir() (string, error) {
	dir := strings.TrimSpace(os.Getenv(updaterHelperTempDirEnv))
	if dir == "" {
		return "", nil
	}

	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve helper temp directory: %w", err)
	}
	if !strings.HasPrefix(filepath.Base(absolute), updaterHelperTempPrefix) {
		return "", fmt.Errorf("refusing to clean unexpected helper directory: %s", absolute)
	}

	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return "", fmt.Errorf("resolve system temp directory: %w", err)
	}
	if !pathWithin(absolute, tempRoot) || samePath(absolute, tempRoot) {
		return "", fmt.Errorf("helper directory is outside system temp: %s", absolute)
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve helper executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve helper executable path: %w", err)
	}
	if !samePath(filepath.Dir(executable), absolute) {
		return "", fmt.Errorf("helper executable is not inside declared temp directory")
	}
	return absolute, nil
}
