package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxScriptSize = 64 << 20 // 64 MiB

func main() {
	if err := run(os.Args[1:], os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "updater:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader) error {
	if len(args) < 3 || len(args) > 4 {
		return fmt.Errorf("usage: %s <pid> <install-root> <patch-root> [restart-path] < script", filepath.Base(os.Args[0]))
	}

	pid, err := strconv.Atoi(args[0])
	if err != nil || pid < 0 {
		return fmt.Errorf("invalid pid: %s", args[0])
	}

	installRoot, err := filepath.Abs(args[1])
	if err != nil {
		return fmt.Errorf("resolve install root: %w", err)
	}
	patchRoot, err := filepath.Abs(args[2])
	if err != nil {
		return fmt.Errorf("resolve patch root: %w", err)
	}
	if samePath(installRoot, patchRoot) {
		return fmt.Errorf("patch root must not equal install root")
	}

	restartPath := ""
	if len(args) == 4 && strings.TrimSpace(args[3]) != "" {
		restartPath, err = filepath.Abs(args[3])
		if err != nil {
			return fmt.Errorf("resolve restart path: %w", err)
		}
	}

	limited := io.LimitReader(stdin, maxScriptSize+1)
	script, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read update script from stdin: %w", err)
	}
	if len(script) == 0 || len(bytes.TrimSpace(script)) == 0 {
		return fmt.Errorf("update script is empty")
	}
	if len(script) > maxScriptSize {
		return fmt.Errorf("update script exceeds %d bytes", maxScriptSize)
	}

	// Read the entire script before starting the interpreter. The parent app can
	// safely exit as soon as this helper has consumed stdin; execution no longer
	// depends on the parent process or a script file on disk.
	runtime := runtimeContext{
		PID:         pid,
		InstallRoot: installRoot,
		PatchRoot:   patchRoot,
		RestartPath: restartPath,
	}
	return runUpdateScript(script, runtime)
}

type runtimeContext struct {
	PID         int
	InstallRoot string
	PatchRoot   string
	RestartPath string
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
