package simpleupdater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareArchiveUpdate(t *testing.T) {
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	write := func(name string, data []byte) {
		t.Helper()
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	write("manifest.json", []byte(`[{"path":"app.exe","size":3}]`))
	write("app.exe", []byte("new"))
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	patchRoot, script, err := prepareArchiveUpdate(bytes.NewReader(archive.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(patchRoot) })

	data, err := os.ReadFile(filepath.Join(patchRoot, "app.exe"))
	if err != nil || string(data) != "new" {
		t.Fatalf("staged app = %q, err = %v", data, err)
	}
	if len(script) == 0 {
		t.Fatal("expected generated update script")
	}
}
