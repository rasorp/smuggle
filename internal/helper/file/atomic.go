package file

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite atomically persists data to path by writing to a temporary file
// in the same directory and then renaming it over the target. Using the same
// directory guarantees the temp file and the target are on the same filesystem,
// which is required for an atomic rename on all supported platforms. Parent
// directories are created if they do not already exist.
func AtomicWrite(path string, data []byte, perms os.FileMode) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".atomic-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmp.Name()

	// Always clean up the temp file on failure; the deferred remove is a
	// no-op once the rename succeeds because the path no longer exists.
	defer func() { _ = os.Remove(tmpPath) }()

	// Set the permissions on the temp file before writing, so that if the
	// target already exists with different permissions, the rename will
	// atomically update them along with the content.
	if err := tmp.Chmod(perms); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to set permissions on temporary file: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename %q to %q: %w", tmpPath, path, err)
	}

	// Ensure the permissions on the target file are correct after the rename.
	// On some platforms, the permissions of the renamed file may be modified by
	// the OS (e.g. umask on Unix), so we set them explicitly here to be sure.
	if err := os.Chmod(path, perms); err != nil {
		return fmt.Errorf("failed to set permissions on target file: %w", err)
	}

	return nil
}

// Delete removes the file at path. It returns nil if the file does not exist,
// making it safe to call unconditionally during teardown.
func Delete(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}
