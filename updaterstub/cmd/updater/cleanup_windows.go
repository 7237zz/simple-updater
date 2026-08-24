//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func cleanupTemporaryHelper() error {
	dir, err := temporaryHelperDir()
	if err != nil || dir == "" {
		return err
	}

	const cleanupScript = `$parentPid = [int]$env:SIMPLE_UPDATER_CLEANUP_PID
$dir = $env:SIMPLE_UPDATER_CLEANUP_DIR
try { Wait-Process -Id $parentPid -ErrorAction SilentlyContinue } catch {}
for ($i = 0; $i -lt 20; $i++) {
    try {
        Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction Stop
        exit 0
    } catch {
        Start-Sleep -Milliseconds 250
    }
}
exit 0`

	cmd := exec.Command(
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", cleanupScript,
	)
	cmd.Env = append(os.Environ(),
		"SIMPLE_UPDATER_CLEANUP_PID="+strconv.Itoa(os.Getpid()),
		"SIMPLE_UPDATER_CLEANUP_DIR="+dir,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start delayed helper cleanup: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release delayed helper cleanup: %w", err)
	}
	return nil
}
