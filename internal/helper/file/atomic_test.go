package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test/must"
)

func TestAtomicWrite(t *testing.T) {
	testCases := []struct {
		name               string
		relPath            string
		data               []byte
		perms              os.FileMode
		preExistingContent []byte
		preExistingPerms   os.FileMode
		expectedError      bool
	}{
		{
			name:               "write new file",
			relPath:            "out.txt",
			data:               []byte("hello"),
			perms:              0644,
			preExistingContent: nil,
			preExistingPerms:   0,
			expectedError:      false,
		},
		{
			name:               "write empty data",
			relPath:            "empty.txt",
			data:               []byte{},
			perms:              0600,
			preExistingContent: nil,
			preExistingPerms:   0,
			expectedError:      false,
		},
		{
			name:               "overwrite existing file",
			relPath:            "existing.txt",
			data:               []byte("new content"),
			perms:              0644,
			preExistingContent: []byte("old content"),
			preExistingPerms:   0644,
			expectedError:      false,
		},
		{
			name:               "creates parent directories",
			relPath:            filepath.Join("a", "b", "c", "file.txt"),
			data:               []byte("nested"),
			perms:              0644,
			preExistingContent: nil,
			preExistingPerms:   0,
			expectedError:      false,
		},
		{
			name:               "restrictive permissions",
			relPath:            "secret.txt",
			data:               []byte("secret"),
			perms:              0600,
			preExistingContent: nil,
			preExistingPerms:   0,
			expectedError:      false,
		},
		{
			name:               "executable permissions",
			relPath:            "script.sh",
			data:               []byte("#!/bin/sh\n"),
			perms:              0755,
			preExistingContent: nil,
			preExistingPerms:   0,
			expectedError:      false,
		},
		{
			name:               "overwrite updates permissions",
			relPath:            "perms.txt",
			data:               []byte("updated"),
			perms:              0600,
			preExistingContent: []byte("original"),
			preExistingPerms:   0644,
			expectedError:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			targetPath := filepath.Join(t.TempDir(), tc.relPath)

			if tc.preExistingContent != nil {
				must.NoError(t, os.WriteFile(targetPath, tc.preExistingContent, tc.preExistingPerms))
			}

			err := AtomicWrite(targetPath, tc.data, tc.perms)

			if tc.expectedError {
				must.Error(t, err)
				return
			} else {
				must.NoError(t, err)

				got, readErr := os.ReadFile(targetPath)
				must.NoError(t, readErr)
				must.Eq(t, tc.data, got)

				info, statErr := os.Stat(targetPath)
				must.NoError(t, statErr)
				must.Eq(t, tc.perms, info.Mode().Perm())

				// No stale temp files should remain alongside the target.
				entries, dirErr := os.ReadDir(filepath.Dir(targetPath))
				must.NoError(t, dirErr)
				for _, e := range entries {
					must.False(t, len(e.Name()) > 8 && e.Name()[:8] == ".atomic-",
						must.Sprintf("stale temp file left behind: %s", e.Name()))
				}
			}
		})
	}
}

func TestDelete(t *testing.T) {
	testCases := []struct {
		name          string
		createFile    bool
		expectedError bool
	}{
		{
			name:       "existing file",
			createFile: true,
		},
		{
			name:          "missing file",
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			target := filepath.Join(t.TempDir(), "target")

			if tc.createFile {
				must.NoError(t, os.WriteFile(target, []byte("data"), 0644))
			}

			actualError := Delete(target)

			if tc.expectedError {
				must.Error(t, actualError)
			} else {
				must.NoError(t, actualError)
				_, statErr := os.Stat(target)
				must.True(t, os.IsNotExist(statErr))
			}
		})
	}
}
