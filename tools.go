package simpleupdater

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

func AnalyzePackage(file SetupReader) (string, PackageType, error) {
	if file == nil {
		return "", "", fmt.Errorf("file is nil")
	}

	const (
		dosHeaderSize   = 64
		peOffsetAddress = 0x3c
		dmgFooterSize   = 512
	)

	size, err := GenerateSize(file)
	if err != nil {
		return "", "", err
	}

	if size >= dosHeaderSize {
		var header [dosHeaderSize]byte
		if _, err := file.ReadAt(header[:], 0); err == nil && bytes.Equal(header[:2], []byte("MZ")) {
			offset := int64(binary.LittleEndian.Uint32(header[peOffsetAddress:]))
			var pe [4]byte
			if _, err := file.ReadAt(pe[:], offset); err == nil && bytes.Equal(pe[:], []byte("PE\x00\x00")) {
				return "windows", PackageTypeInno, nil
			}
		}
	}

	if size >= dmgFooterSize {
		var footer [dmgFooterSize]byte
		if _, err := file.ReadAt(footer[:], size-dmgFooterSize); err == nil && bytes.Equal(footer[:4], []byte("koly")) {
			return "darwin", PackageTypeDMG, nil
		}
	}

	return "", "", fmt.Errorf("unsupported setup package")
}

func AnalyzeSystem(file SetupReader) (string, error) {
	system, _, err := AnalyzePackage(file)
	return system, err
}

func GenerateSHA256(file SetupReader) (string, error) {
	if file == nil {
		return "", fmt.Errorf("file is nil")
	}

	size, err := GenerateSize(file)
	if err != nil {
		return "", err
	}
	return generateSHA256(file, size)
}

func generateSHA256(file io.ReaderAt, size int64) (string, error) {
	if size < 0 {
		return "", fmt.Errorf("invalid file size: %d", size)
	}

	hash := sha256.New()
	reader := io.NewSectionReader(file, 0, size)
	if _, err := io.Copy(hash, reader); err != nil {
		return "", fmt.Errorf("calculate SHA256: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func GenerateSize(file io.Seeker) (int64, error) {
	if file == nil {
		return 0, fmt.Errorf("file is nil")
	}

	current, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("get current file offset: %w", err)
	}

	size, endErr := file.Seek(0, io.SeekEnd)
	_, restoreErr := file.Seek(current, io.SeekStart)
	if endErr != nil {
		return 0, fmt.Errorf("seek file end: %w", endErr)
	}
	if restoreErr != nil {
		return 0, fmt.Errorf("restore file offset: %w", restoreErr)
	}

	return size, nil
}
