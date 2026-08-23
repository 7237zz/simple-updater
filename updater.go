package simpleupdater

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func ReadProductManifest(root string) ([]File, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat project directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project path is not a directory: %s", root)
	}

	files := make([]File, 0)
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(root, filePath)
		if err != nil {
			return fmt.Errorf("get relative path for %s: %w", filePath, err)
		}
		relativePath = filepath.ToSlash(relativePath)

		fileInfo, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", filePath, err)
		}

		if fileInfo.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filePath)
			if err != nil {
				return fmt.Errorf("read symlink %s: %w", filePath, err)
			}
			files = append(files, File{
				Path:       relativePath,
				Type:       FileTypeSymlink,
				Mode:       uint32(fileInfo.Mode().Perm()),
				LinkTarget: target,
			})
			return nil
		}

		if !fileInfo.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("open %s: %w", filePath, err)
		}
		sha256, hashErr := GenerateSHA256(file)
		closeErr := file.Close()
		if hashErr != nil {
			return fmt.Errorf("hash %s: %w", filePath, hashErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", filePath, closeErr)
		}

		files = append(files, File{
			Path:   relativePath,
			Type:   FileTypeRegular,
			Size:   uint64(fileInfo.Size()),
			SHA256: sha256,
			Mode:   uint32(fileInfo.Mode().Perm()),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}
