package updaterstub

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
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

// UniversalBuildOptions controls creation of one macOS Universal updater that
// contains both amd64 and arm64 Mach-O slices.
type UniversalBuildOptions struct {
	Output  string
	Package string
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

	output, workDir, err := resolveOutputAndWorkDir(options.Output, options.WorkDir, system == "windows")
	if err != nil {
		return "", err
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

// BuildUniversalDarwin builds amd64 and arm64 updater shells and merges them
// into one macOS Universal (fat Mach-O) executable. The merge is implemented in
// Go and does not require lipo, Xcode, or a macOS build host.
func BuildUniversalDarwin(ctx context.Context, options UniversalBuildOptions) (string, error) {
	if ctx == nil {
		return "", errors.New("context is nil")
	}

	output, workDir, err := resolveOutputAndWorkDir(options.Output, options.WorkDir, false)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(options.Output) == "" {
		if workDir != "" {
			output = filepath.Join(workDir, "updater-darwin-universal")
		} else {
			output, err = filepath.Abs("updater-darwin-universal")
			if err != nil {
				return "", fmt.Errorf("resolve universal updater output: %w", err)
			}
		}
	}

	tempDir, err := os.MkdirTemp("", "simple-updater-universal-build-")
	if err != nil {
		return "", fmt.Errorf("create universal build directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	amd64Path, err := Build(ctx, BuildOptions{
		System:  "darwin",
		Arch:    "amd64",
		Output:  filepath.Join(tempDir, "updater-darwin-amd64"),
		Package: options.Package,
		WorkDir: workDir,
	})
	if err != nil {
		return "", err
	}
	arm64Path, err := Build(ctx, BuildOptions{
		System:  "darwin",
		Arch:    "arm64",
		Output:  filepath.Join(tempDir, "updater-darwin-arm64"),
		Package: options.Package,
		WorkDir: workDir,
	})
	if err != nil {
		return "", err
	}

	if err := mergeMachOUniversal(output, amd64Path, arm64Path); err != nil {
		return "", fmt.Errorf("create universal updater: %w", err)
	}
	return output, nil
}

func resolveOutputAndWorkDir(output, workDir string, windows bool) (string, string, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir != "" {
		absolute, err := filepath.Abs(workDir)
		if err != nil {
			return "", "", fmt.Errorf("resolve work dir: %w", err)
		}
		workDir = absolute
	}

	output = strings.TrimSpace(output)
	if output == "" {
		output = "updater"
		if windows {
			output += ".exe"
		}
	}
	if !filepath.IsAbs(output) && workDir != "" {
		output = filepath.Join(workDir, output)
	}
	absoluteOutput, err := filepath.Abs(output)
	if err != nil {
		return "", "", fmt.Errorf("resolve output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absoluteOutput), 0o755); err != nil {
		return "", "", fmt.Errorf("create output directory: %w", err)
	}
	return absoluteOutput, workDir, nil
}

type machoSlice struct {
	path   string
	cpu    uint32
	subCPU uint32
	offset uint32
	size   uint32
}

func mergeMachOUniversal(output string, thinPaths ...string) error {
	if len(thinPaths) < 2 {
		return errors.New("at least two Mach-O slices are required")
	}

	const (
		fatMagic      = uint32(0xcafebabe)
		fatArchSize   = uint64(20)
		fatHeaderSize = uint64(8)
		alignExponent = uint32(14) // 16 KiB, accepted by macOS fat Mach-O tooling.
		alignment     = uint64(1) << alignExponent
	)

	slices := make([]machoSlice, 0, len(thinPaths))
	seenCPU := make(map[uint32]struct{}, len(thinPaths))
	for _, path := range thinPaths {
		file, err := macho.Open(path)
		if err != nil {
			return fmt.Errorf("open Mach-O slice %s: %w", path, err)
		}
		cpu := uint32(file.Cpu)
		subCPU := file.SubCpu
		magic := file.Magic
		_ = file.Close()
		if magic != macho.Magic64 {
			return fmt.Errorf("Mach-O slice %s is not 64-bit", path)
		}
		if fileCPU := macho.Cpu(cpu); fileCPU != macho.CpuAmd64 && fileCPU != macho.CpuArm64 {
			return fmt.Errorf("unsupported Mach-O cpu in %s: %s", path, fileCPU)
		}
		if _, exists := seenCPU[cpu]; exists {
			return fmt.Errorf("duplicate Mach-O cpu slice in %s", path)
		}
		seenCPU[cpu] = struct{}{}

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat Mach-O slice %s: %w", path, err)
		}
		if info.Size() <= 0 || info.Size() > math.MaxUint32 {
			return fmt.Errorf("Mach-O slice size is unsupported for %s: %d", path, info.Size())
		}
		slices = append(slices, machoSlice{
			path:   path,
			cpu:    cpu,
			subCPU: subCPU,
			size:   uint32(info.Size()),
		})
	}

	headerEnd := fatHeaderSize + uint64(len(slices))*fatArchSize
	nextOffset := alignUp(headerEnd, alignment)
	for i := range slices {
		if nextOffset > math.MaxUint32 {
			return errors.New("universal Mach-O offset exceeds 32-bit fat format")
		}
		slices[i].offset = uint32(nextOffset)
		nextOffset = alignUp(nextOffset+uint64(slices[i].size), alignment)
	}
	if nextOffset > math.MaxUint32 {
		return errors.New("universal Mach-O exceeds 32-bit fat format")
	}

	temp, err := os.CreateTemp(filepath.Dir(output), ".updater-universal-*")
	if err != nil {
		return fmt.Errorf("create universal output: %w", err)
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	writeUint32 := func(value uint32) error {
		return binary.Write(temp, binary.BigEndian, value)
	}
	if err := writeUint32(fatMagic); err != nil {
		return err
	}
	if err := writeUint32(uint32(len(slices))); err != nil {
		return err
	}
	for _, slice := range slices {
		for _, value := range []uint32{slice.cpu, slice.subCPU, slice.offset, slice.size, alignExponent} {
			if err := writeUint32(value); err != nil {
				return err
			}
		}
	}

	position := headerEnd
	for _, slice := range slices {
		if uint64(slice.offset) < position {
			return errors.New("invalid universal Mach-O slice offset")
		}
		if err := writeZeroPadding(temp, uint64(slice.offset)-position); err != nil {
			return err
		}
		source, err := os.Open(slice.path)
		if err != nil {
			return fmt.Errorf("open Mach-O slice %s: %w", slice.path, err)
		}
		written, copyErr := io.Copy(temp, source)
		closeErr := source.Close()
		if copyErr != nil {
			return fmt.Errorf("copy Mach-O slice %s: %w", slice.path, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close Mach-O slice %s: %w", slice.path, closeErr)
		}
		if written != int64(slice.size) {
			return fmt.Errorf("Mach-O slice size changed while merging %s", slice.path)
		}
		position = uint64(slice.offset) + uint64(slice.size)
	}

	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync universal updater: %w", err)
	}
	if err := temp.Chmod(0o755); err != nil {
		return fmt.Errorf("chmod universal updater: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close universal updater: %w", err)
	}
	if err := os.Remove(output); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace existing universal updater: %w", err)
	}
	if err := os.Rename(tempPath, output); err != nil {
		return fmt.Errorf("commit universal updater: %w", err)
	}
	committed = true

	fat, err := macho.OpenFat(output)
	if err != nil {
		return fmt.Errorf("verify universal updater: %w", err)
	}
	defer fat.Close()
	if len(fat.Arches) != len(slices) {
		return fmt.Errorf("verify universal updater: expected %d slices, got %d", len(slices), len(fat.Arches))
	}
	return nil
}

func alignUp(value, alignment uint64) uint64 {
	return (value + alignment - 1) &^ (alignment - 1)
}

func writeZeroPadding(writer io.Writer, size uint64) error {
	zero := make([]byte, 32*1024)
	for size > 0 {
		chunk := uint64(len(zero))
		if size < chunk {
			chunk = size
		}
		if _, err := writer.Write(zero[:int(chunk)]); err != nil {
			return err
		}
		size -= chunk
	}
	return nil
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
