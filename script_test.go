package simpleupdater

import (
	"strings"
	"testing"
)

func TestGenerateUpdateScriptValidatesManifest(t *testing.T) {
	if _, err := GenerateUpdateScript("windows", []File{{Path: "../app.exe"}}); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}

	script, err := GenerateUpdateScript("darwin", []File{{
		Path:   "Contents/MacOS/App",
		Size:   3,
		SHA256: "abc",
		Mode:   0o755,
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Contents/MacOS/App", "abc", "chmod"} {
		if !strings.Contains(script, want) {
			t.Fatalf("generated script does not contain %q", want)
		}
	}
}
