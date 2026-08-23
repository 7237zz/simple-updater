//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func runUpdateScript(script []byte, runtime runtimeContext) error {
	cmd := exec.Command(
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", "-",
	)
	cmd.Stdin = bytes.NewReader(script)
	cmd.Dir = runtime.InstallRoot
	cmd.Env = append(os.Environ(),
		"SIMPLE_UPDATER_PID="+strconv.Itoa(runtime.PID),
		"SIMPLE_UPDATER_INSTALL_ROOT="+runtime.InstallRoot,
		"SIMPLE_UPDATER_PATCH_ROOT="+runtime.PatchRoot,
		"SIMPLE_UPDATER_RESTART_PATH="+runtime.RestartPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("powershell update failed: %w: %s", err, output)
	}
	return nil
}
