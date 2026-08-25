package simpleupdater

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"
)

func TestInMemorySetupReader(t *testing.T) {
	data := make([]byte, 128)
	copy(data[:2], "MZ")
	binary.LittleEndian.PutUint32(data[0x3c:], 0x40)
	copy(data[0x40:], "PE\x00\x00")
	setup := bytes.NewReader(data)

	system, packageType, err := AnalyzePackage(setup)
	if err != nil {
		t.Fatal(err)
	}
	if system != "windows" || packageType != PackageTypeInno {
		t.Fatalf("got %s/%s, want windows/%s", system, packageType, PackageTypeInno)
	}

	size, err := GenerateSize(setup)
	if err != nil || size != int64(len(data)) {
		t.Fatalf("size = %d, err = %v", size, err)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(data))
	gotHash, err := GenerateSHA256(setup)
	if err != nil || gotHash != wantHash {
		t.Fatalf("hash = %q, err = %v; want %q", gotHash, err, wantHash)
	}

	name, err := GenerateSetupFileName(&Product{
		Product:     "Demo App",
		Version:     "1.2",
		System:      system,
		PackageType: packageType,
	})
	if err != nil || name != "Demo-App-1.2-windows-setup.exe" {
		t.Fatalf("name = %q, err = %v", name, err)
	}
}
