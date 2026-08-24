package simpleupdater

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxUpdateManifestSize = 64 << 20 // 64 MiB

// ExecuteUpdate starts a complete self-update handoff using only the updater
// helper path and the prepared temporary update directory.
//
// The update directory must contain manifest.json plus the update payload files
// at the same relative paths described by the manifest. ExecuteUpdate:
//  1. resolves the current application's install root and restart target;
//  2. reads and validates <updateRoot>/manifest.json;
//  3. generates the current platform's update script entirely in memory;
//  4. copies updater to an isolated system temporary directory and streams the
//     complete script to that helper over stdin;
//  5. terminates the current application after updater has received the script.
//
// On Windows, the current executable's directory is treated as InstallRoot and
// the executable itself is restarted. On macOS, ExecuteUpdate locates the
// enclosing .app bundle and uses the bundle root as both InstallRoot and the
// restart target.
//
// ExecuteUpdate returns only when preparation or updater handoff fails. On a
// successful handoff it terminates the current process with exit code 0 so the
// updater can safely replace files that were in use by this application.
func ExecuteUpdate(updaterPath, updateRoot string) error {
	if _, err := prepareUpdateHandoff(updaterPath, updateRoot); err != nil {
		return err
	}

	// StartUpdater does not return until the updater helper has consumed the
	// complete generated script from stdin. It is therefore safe to terminate
	// immediately here without risking a truncated script.
	os.Exit(0)
	return nil // unreachable; keeps the function convenient to call as error-returning API.
}

func prepareUpdateHandoff(updaterPath, updateRoot string) (int, error) {
	if strings.TrimSpace(updaterPath) == "" {
		return 0, errors.New("updater path is empty")
	}
	if strings.TrimSpace(updateRoot) == "" {
		return 0, errors.New("update root is empty")
	}

	updaterPath, err := filepath.Abs(updaterPath)
	if err != nil {
		return 0, fmt.Errorf("resolve updater path: %w", err)
	}
	updaterInfo, err := os.Stat(updaterPath)
	if err != nil {
		return 0, fmt.Errorf("stat updater: %w", err)
	}
	if updaterInfo.IsDir() {
		return 0, errors.New("updater path points to a directory")
	}

	updateRoot, err = filepath.Abs(updateRoot)
	if err != nil {
		return 0, fmt.Errorf("resolve update root: %w", err)
	}
	updateInfo, err := os.Stat(updateRoot)
	if err != nil {
		return 0, fmt.Errorf("stat update root: %w", err)
	}
	if !updateInfo.IsDir() {
		return 0, errors.New("update root is not a directory")
	}

	installRoot, restartPath, err := currentUpdateTarget()
	if err != nil {
		return 0, err
	}
	if sameFilesystemPath(installRoot, updateRoot) {
		return 0, errors.New("update root must not equal install root")
	}
	if pathWithin(updaterPath, updateRoot) {
		return 0, errors.New("updater executable must not be inside update root")
	}

	manifestPath := filepath.Join(updateRoot, "manifest.json")
	manifestData, err := readLimitedFile(manifestPath, maxUpdateManifestSize)
	if err != nil {
		return 0, fmt.Errorf("read update manifest: %w", err)
	}

	var manifest []File
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return 0, fmt.Errorf("unmarshal update manifest: %w", err)
	}
	if len(manifest) == 0 {
		return 0, errors.New("update manifest is empty")
	}

	script, err := GenerateUpdateScript(runtime.GOOS, manifest)
	if err != nil {
		return 0, fmt.Errorf("generate update script: %w", err)
	}

	pid, err := StartUpdater(UpdaterLaunchOptions{
		UpdaterPath: updaterPath,
		PID:         os.Getpid(),
		InstallRoot: installRoot,
		PatchRoot:   updateRoot,
		RestartPath: restartPath,
		Script:      []byte(script),
	})
	if err != nil {
		return 0, fmt.Errorf("start updater: %w", err)
	}
	return pid, nil
}

func currentUpdateTarget() (installRoot, restartPath string, err error) {
	executable, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("resolve current executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", "", fmt.Errorf("resolve current executable path: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}

	switch runtime.GOOS {
	case "windows":
		return filepath.Dir(executable), executable, nil
	case "darwin":
		appRoot, err := enclosingAppBundle(executable)
		if err != nil {
			return "", "", err
		}
		return appRoot, appRoot, nil
	default:
		return "", "", fmt.Errorf("self update is unsupported on %s", runtime.GOOS)
	}
}

func enclosingAppBundle(executable string) (string, error) {
	current := filepath.Dir(executable)
	for {
		if strings.EqualFold(filepath.Ext(current), ".app") {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("current executable is not inside a .app bundle: %s", executable)
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("manifest is not a regular file")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("manifest exceeds %d bytes", limit)
	}

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("manifest exceeds %d bytes", limit)
	}
	return data, nil
}
