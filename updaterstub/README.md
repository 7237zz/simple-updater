# updaterstub

A tiny build-time helper for producing platform-specific updater shells.

The application generates an update script in memory, copies the updater shell
to an isolated system temporary directory, starts that temporary helper, writes
the script to updater stdin, then exits. Because the running helper is no longer
the updater stored in the application install directory, the installed updater
binary can be replaced by the same update transaction.

Runtime usage:

```text
updater[.exe] <pid> <install-root> <patch-root> [restart-path] < script
```

The updater passes runtime values to the script through environment variables:

```text
SIMPLE_UPDATER_PID
SIMPLE_UPDATER_INSTALL_ROOT
SIMPLE_UPDATER_PATCH_ROOT
SIMPLE_UPDATER_RESTART_PATH
```

Windows executes stdin with Windows PowerShell. macOS executes stdin with
`/bin/sh -s`. No `.ps1` or `.sh` file needs to be created in the temporary
update directory.

The temporary helper cleans itself after the update finishes. On macOS the
helper directory can be removed directly. On Windows a hidden PowerShell cleanup
process waits for the helper process to exit and then removes the temporary
helper directory.

## Build

Build one architecture with `Build`. Build a macOS Universal updater containing
both amd64 and arm64 slices with `BuildUniversalDarwin`. Universal merging is
implemented in Go and does not require `lipo` or Xcode.

From the repository root, build the release helpers with:

```text
go run ./cmd/build-updaters
```

Outputs:

```text
dist/updater-windows-amd64.exe
dist/updater-darwin-universal
```
