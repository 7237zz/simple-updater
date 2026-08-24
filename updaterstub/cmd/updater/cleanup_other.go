//go:build !windows && !darwin

package main

import "os"

func cleanupTemporaryHelper() error {
	dir, err := temporaryHelperDir()
	if err != nil || dir == "" {
		return err
	}
	return os.RemoveAll(dir)
}
