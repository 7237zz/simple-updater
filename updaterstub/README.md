# updaterstub

A tiny build-time helper for producing a platform-specific updater shell.

The shell has no manifest or patch logic. The application generates an update
script in memory, starts the updater, writes the script to updater stdin, then
exits. The updater first reads the complete script into memory and only then
starts PowerShell or `/bin/sh`.

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
update directory, so a successful update can delete that directory completely.
