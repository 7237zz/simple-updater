package simpleupdater

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

func (p PackageType) System() string {
	switch p {
	case PackageTypeInno:
		return "windows"
	case PackageTypeDMG:
		return "darwin"
	default:
		return ""
	}
}

func (p PackageType) Extension() string {
	switch p {
	case PackageTypeInno:
		return ".exe"
	case PackageTypeDMG:
		return ".dmg"
	default:
		return ""
	}
}

func GenerateSetupFileName(product *Product) (string, error) {
	if product == nil {
		return "", errors.New("product is nil")
	}

	name := sanitizeFileNamePart(product.Product)
	if name == "" {
		return "", errors.New("product name is empty")
	}
	version := sanitizeFileNamePart(product.Version)
	if version == "" {
		return "", errors.New("product version is empty")
	}
	if product.System == "" {
		return "", errors.New("product system is empty")
	}

	expectedSystem := product.PackageType.System()
	ext := product.PackageType.Extension()
	if expectedSystem == "" || ext == "" {
		return "", fmt.Errorf("unsupported package type: %s", product.PackageType)
	}
	if product.System != expectedSystem {
		return "", fmt.Errorf("package type %s is not valid for system %s", product.PackageType, product.System)
	}

	return fmt.Sprintf("%s-%s-%s-setup%s", name, version, product.System, ext), nil
}

func sanitizeFileNamePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))
	lastSeparator := false

	for _, r := range value {
		invalid := unicode.IsControl(r) || unicode.IsSpace(r) || strings.ContainsRune(`<>:"/\\|?*`, r)
		if invalid {
			if b.Len() > 0 && !lastSeparator {
				b.WriteByte('-')
				lastSeparator = true
			}
			continue
		}

		b.WriteRune(r)
		lastSeparator = r == '-'
	}

	return strings.Trim(b.String(), " .-")
}
