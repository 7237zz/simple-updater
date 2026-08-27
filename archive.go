package simpleupdater

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const updaterPatchTempPrefix = "simple-updater-patch-"

func prepareArchiveUpdate(archive io.Reader) (string, []byte, error) {
	if archive == nil {
		return "", nil, errors.New("update archive is nil")
	}

	patchRoot, err := os.MkdirTemp("", updaterPatchTempPrefix)
	if err != nil {
		return "", nil, fmt.Errorf("create temporary patch root: %w", err)
	}
	removePatch := true
	defer func() {
		if removePatch {
			_ = os.RemoveAll(patchRoot)
		}
	}()

	if err := extractArchive(archive, patchRoot); err != nil {
		return "", nil, err
	}

	manifestPath := filepath.Join(patchRoot, "manifest.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return "", nil, fmt.Errorf("stat patch manifest: %w", err)
	}
	if !manifestInfo.Mode().IsRegular() {
		return "", nil, errors.New("patch manifest is not a regular file")
	}
	manifestData, err := readLimitedFile(manifestPath, maxUpdateManifestSize)
	if err != nil {
		return "", nil, fmt.Errorf("read patch manifest: %w", err)
	}

	var manifest []File
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return "", nil, fmt.Errorf("unmarshal patch manifest: %w", err)
	}
	if len(manifest) == 0 {
		return "", nil, errors.New("patch manifest is empty")
	}

	script, err := GenerateUpdateScript(runtime.GOOS, manifest)
	if err != nil {
		return "", nil, fmt.Errorf("generate update script: %w", err)
	}
	removePatch = false
	return patchRoot, []byte(script), nil
}

func extractArchive(archive io.Reader, destinationRoot string) error {
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open patch gzip: %w", err)
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	seen := make(map[string]struct{})
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read patch archive: %w", err)
		}

		relative, err := cleanArchiveName(header.Name)
		if err != nil {
			return err
		}
		key := relative
		if runtime.GOOS == "windows" {
			key = strings.ToLower(filepath.FromSlash(key))
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate patch entry: %s", header.Name)
		}
		seen[key] = struct{}{}

		destination := filepath.Join(destinationRoot, filepath.FromSlash(relative))
		if err := ensureArchiveParent(destinationRoot, destination); err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return fmt.Errorf("invalid patch file size: %s", relative)
			}
			mode := os.FileMode(header.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return fmt.Errorf("create patch file %s: %w", relative, err)
			}
			_, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("extract patch file %s: %w", relative, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close patch file %s: %w", relative, closeErr)
			}
			if err := os.Chmod(destination, mode); err != nil {
				return fmt.Errorf("set patch file mode %s: %w", relative, err)
			}
		case tar.TypeSymlink:
			if header.Size != 0 {
				return fmt.Errorf("symlink entry has data: %s", relative)
			}
			if err := validateArchiveSymlink(relative, header.Linkname); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, destination); err != nil {
				return fmt.Errorf("create patch symlink %s: %w", relative, err)
			}
		default:
			return fmt.Errorf("unsupported patch entry type for %s", relative)
		}
	}
}

func ensureArchiveParent(root, target string) error {
	parent := filepath.Dir(target)
	relative, err := filepath.Rel(root, parent)
	if err != nil {
		return fmt.Errorf("resolve patch parent: %w", err)
	}
	if relative == "." {
		return nil
	}

	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return fmt.Errorf("create patch directory %s: %w", part, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("stat patch directory %s: %w", part, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("patch path parent is not a directory: %s", current)
		}
	}
	return nil
}

func validateArchiveSymlink(name, target string) error {
	if err := validateSymlinkTarget(name, target); err != nil {
		return err
	}
	targetPath := filepath.FromSlash(target)
	if filepath.IsAbs(targetPath) || filepath.VolumeName(targetPath) != "" {
		return fmt.Errorf("invalid patch symlink target for %s: %s", name, target)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(filepath.FromSlash(name)), targetPath))
	if resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return fmt.Errorf("patch symlink target escapes root for %s: %s", name, target)
	}
	return nil
}
