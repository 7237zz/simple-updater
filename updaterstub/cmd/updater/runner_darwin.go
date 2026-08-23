//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

func runUpdateScript(script []byte, runtime runtimeContext) error {
	cmd := exec.Command("/bin/sh", "-s")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Dir = runtime.InstallRoot
	cmd.Env = append(os.Environ(),
		"SIMPLE_UPDATER_PID="+strconv.Itoa(runtime.PID),
		"SIMPLE_UPDATER_INSTALL_ROOT="+runtime.InstallRoot,
		"SIMPLE_UPDATER_PATCH_ROOT="+runtime.PatchRoot,
		"SIMPLE_UPDATER_RESTART_PATH="+runtime.RestartPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("shell update failed: %w: %s", err, output)
	}
	return nil
}
