package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Peiratooo/simple-updater/updaterstub"
)

func main() {
	ctx := context.Background()
	root, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}

	windowsOutput, err := updaterstub.Build(ctx, updaterstub.BuildOptions{
		System:  "windows",
		Arch:    "amd64",
		Output:  filepath.Join("dist", "updater-windows-amd64.exe"),
		WorkDir: root,
	})
	if err != nil {
		log.Fatalf("build windows updater: %v", err)
	}
	fmt.Printf("built windows/amd64 updater: %s\n", windowsOutput)

	macOutput, err := updaterstub.BuildUniversalDarwin(ctx, updaterstub.UniversalBuildOptions{
		Output:  filepath.Join("dist", "updater-darwin-universal"),
		WorkDir: root,
	})
	if err != nil {
		log.Fatalf("build macOS universal updater: %v", err)
	}
	fmt.Printf("built darwin universal updater (amd64 + arm64): %s\n", macOutput)
}
