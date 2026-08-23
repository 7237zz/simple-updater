//go:build !windows && !darwin

package main

import "fmt"

func runUpdateScript([]byte, runtimeContext) error {
	return fmt.Errorf("updater shell only supports windows and darwin")
}
