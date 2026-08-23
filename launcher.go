package simpleupdater

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// UpdaterLaunchOptions describes one handoff from the running application to
// the tiny updater helper. Script stays in memory and is delivered over stdin.
type UpdaterLaunchOptions struct {
	UpdaterPath string
	PID         int
	InstallRoot string
	PatchRoot   string
	RestartPath string
	Script      []byte
}

// StartUpdater starts the updater helper, synchronously writes the complete
// generated update script to its stdin, closes stdin, and then detaches from
// the helper process. Once this function returns successfully, the calling
// application may exit immediately without truncating the update script.
//
// The helper is expected to accept:
//
//	updater[.exe] <pid> <install-root> <patch-root> [restart-path]
//
// and to execute the script received through stdin.
func StartUpdater(options UpdaterLaunchOptions) (int, error) {
	if strings.TrimSpace(options.UpdaterPath) == "" {
		return 0, errors.New("updater path is empty")
	}
	if options.PID < 0 {
		return 0, fmt.Errorf("invalid pid: %d", options.PID)
	}
	if len(options.Script) == 0 || len(strings.TrimSpace(string(options.Script))) == 0 {
		return 0, errors.New("update script is empty")
	}

	updaterPath, err := filepath.Abs(options.UpdaterPath)
	if err != nil {
		return 0, fmt.Errorf("resolve updater path: %w", err)
	}
	installRoot, err := filepath.Abs(options.InstallRoot)
	if err != nil {
		return 0, fmt.Errorf("resolve install root: %w", err)
	}
	patchRoot, err := filepath.Abs(options.PatchRoot)
	if err != nil {
		return 0, fmt.Errorf("resolve patch root: %w", err)
	}
	if sameFilesystemPath(installRoot, patchRoot) {
		return 0, errors.New("patch root must not equal install root")
	}
	if pathWithin(updaterPath, patchRoot) {
		return 0, errors.New("updater executable must not be inside patch root")
	}

	args := []string{
		strconv.Itoa(options.PID),
		installRoot,
		patchRoot,
	}
	if strings.TrimSpace(options.RestartPath) != "" {
		restartPath, err := filepath.Abs(options.RestartPath)
		if err != nil {
			return 0, fmt.Errorf("resolve restart path: %w", err)
		}
		args = append(args, restartPath)
	}

	cmd := exec.Command(updaterPath, args...)
	cmd.Dir = filepath.Dir(updaterPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, fmt.Errorf("open updater stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return 0, fmt.Errorf("start updater: %w", err)
	}

	pid := cmd.Process.Pid
	if _, err := writeAll(stdin, options.Script); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return 0, fmt.Errorf("send update script to updater: %w", err)
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return 0, fmt.Errorf("close updater stdin: %w", err)
	}

	// The helper owns the rest of the update lifecycle. Releasing avoids making
	// the app wait for the update to finish; the app should normally exit now.
	if err := cmd.Process.Release(); err != nil {
		return 0, fmt.Errorf("release updater process: %w", err)
	}
	return pid, nil
}

func writeAll(writer io.Writer, data []byte) (int, error) {
	written := 0
	for written < len(data) {
		n, err := writer.Write(data[written:])
		written += n
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func sameFilesystemPath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func pathWithin(candidate, root string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
