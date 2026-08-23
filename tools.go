package simpleupdater

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func AnalyzeSystem(file *os.File) (string, error) { // 分析系统
	if file == nil {
		return "", fmt.Errorf("file is nil")
	}

	const (
		dosHeaderSize   = 64
		peOffsetAddress = 0x3c
		dmgFooterSize   = 512
	)

	// 读取 DOS header
	var header [dosHeaderSize]byte
	if _, err := file.ReadAt(header[:], 0); err != nil {
		return "", fmt.Errorf("read DOS header: %w", err)
	}

	// 判断 Windows PE (.exe)
	if bytes.Equal(header[:2], []byte("MZ")) {
		// PE header offset
		offset := int64(binary.LittleEndian.Uint32(header[peOffsetAddress:]))

		var pe [4]byte
		if _, err := file.ReadAt(pe[:], offset); err == nil && bytes.Equal(pe[:], []byte("PE\x00\x00")) {
			return "windows", nil
		}
	}

	// 判断 macOS DMG
	// DMG 的 koly 标识在文件尾部，需要获取文件大小
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.Size() >= dmgFooterSize {
		var footer [dmgFooterSize]byte
		if _, err := file.ReadAt(footer[:], info.Size()-dmgFooterSize); err == nil && bytes.Equal(footer[:4], []byte("koly")) {
			return "darwin", nil
		}
	}

	return "", fmt.Errorf("unknown system")
}

func GenerateSHA256(file *os.File) (string, error) {
	if file == nil {
		return "", fmt.Errorf("file is nil")
	}

	// 获取文件大小
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	// 创建 SHA256 hash
	hash := sha256.New()

	// 从文件开头读取，避免改变文件当前偏移
	reader := io.NewSectionReader(file, 0, info.Size())
	if _, err := io.Copy(hash, reader); err != nil {
		return "", fmt.Errorf("calculate SHA256: %w", err)
	}

	// 返回 hex 字符串
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func GenerateSize(file *os.File) (int64, error) {
	if file == nil {
		return 0, fmt.Errorf("file is nil")
	}

	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat file: %w", err)
	}

	return info.Size(), nil
}
