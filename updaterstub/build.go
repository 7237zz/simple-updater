package updaterstub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BuildOptions controls how the updater shell is cross-compiled.
type BuildOptions struct {
	// System accepts "windows" or "darwin" ("mac"/"macos" are aliases).
	System string
	// Arch is the target GOARCH. When empty, the current runtime.GOARCH is used.
	Arch string
	// Output is the final updater executable path. When empty, a platform
	// specific name is created in the current directory.
	Output string
	// Package is the Go package containing the updater shell command.
	// Leave empty when this package lives at ./updaterstub and the command at
	// ./updaterstub/cmd/updater.
	Package string
	// WorkDir is the module root used as the working directory for `go build`.
	// Empty means the current working directory.
	WorkDir string
}

// Build cross-compiles the updater shell for Windows or macOS.
//
// The build uses only the Go standard library and forces CGO_ENABLED=0, so the
// resulting updater has no cgo runtime dependency. A Go toolchain must be
// available on the machine performing the build; end-user machines do not need
// Go installed.
func Build(ctx context.Context, options BuildOptions) (string, error) {
	if ctx == nil {
		return "", errors.New("context is nil")
	}

	system, err := normalizeSystem(options.System)
	if err != nil {
		return "", err
	}

	arch := strings.TrimSpace(options.Arch)
	if arch == "" {
		arch = runtime.GOARCH
	}
	if err := validateArch(arch); err != nil {
		return "", err
	}

	pkg := strings.TrimSpace(options.Package)
	if pkg == "" {
		pkg = "./updaterstub/cmd/updater"
	}

	output := strings.TrimSpace(options.Output)
	if output == "" {
		output = "updater"
		if system == "windows" {
			output += ".exe"
		}
	}

	workDir := strings.TrimSpace(options.WorkDir)
	if workDir != "" {
		absolute, err := filepath.Abs(workDir)
		if err != nil {
			return "", fmt.Errorf("resolve work dir: %w", err)
		}
		workDir = absolute
	}

	if !filepath.IsAbs(output) && workDir != "" {
		output = filepath.Join(workDir, output)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return "", fmt.Errorf("resolve output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	args := []string{"build", "-trimpath", "-ldflags=-s -w"}
	if system == "windows" {
		// The updater is a background helper; avoid opening an extra console.
		args[len(args)-1] = "-ldflags=-s -w -H=windowsgui"
	}
	args = append(args, "-o", output, pkg)

	cmd := exec.CommandContext(ctx, "go", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = withBuildEnv(os.Environ(), system, arch)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return "", fmt.Errorf("build updater for %s/%s: %w", system, arch, err)
		}
		return "", fmt.Errorf("build updater for %s/%s: %w: %s", system, arch, err, message)
	}

	return output, nil
}

func normalizeSystem(system string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(system)) {
	case "windows", "win":
		return "windows", nil
	case "darwin", "mac", "macos":
		return "darwin", nil
	default:
		return "", fmt.Errorf("unsupported updater system: %s", system)
	}
}

func validateArch(arch string) error {
	switch arch {
	case "amd64", "arm64":
		return nil
	default:
		return fmt.Errorf("unsupported updater architecture: %s", arch)
	}
}

func withBuildEnv(base []string, system, arch string) []string {
	env := make([]string, 0, len(base)+3)
	for _, item := range base {
		if strings.HasPrefix(item, "GOOS=") || strings.HasPrefix(item, "GOARCH=") || strings.HasPrefix(item, "CGO_ENABLED=") {
			continue
		}
		env = append(env, item)
	}
	return append(env,
		"GOOS="+system,
		"GOARCH="+arch,
		"CGO_ENABLED=0",
	)
}
