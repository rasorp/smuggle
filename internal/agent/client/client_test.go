package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shoenig/test/must"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/helper"
)

// newTestClient returns the minimum Client needed to exercise generateID.
// Only cfg.DataDir is consulted by that function, so everything else is left
// at its zero value.
func newTestClient(t *testing.T, dataDir string) *Client {
	t.Helper()
	return &Client{
		cfg: &config.ClientConfig{DataDir: dataDir},
	}
}

func TestGenerateID(t *testing.T) {
	const bareID = "e3b0c442-98fc-4c14-b8bf-09e98ce58865"

	testCases := []struct {
		name        string
		fileContent *string
		validate    func(t *testing.T, dir string, c *Client)
	}{
		{
			name:        "fresh directory generates clean UUID",
			fileContent: nil,
			validate: func(t *testing.T, dir string, c *Client) {
				id := c.getID()
				must.NotEq(t, "", id)

				// The stored ID must contain no whitespace.
				must.Eq(t, id, strings.TrimSpace(id))

				// The ID file must exist and contain exactly the stored ID.
				data, err := os.ReadFile(filepath.Join(dir, clientIDFileName))
				must.NoError(t, err)
				must.Eq(t, id, string(data))
			},
		},
		{
			name:        "reads existing clean ID",
			fileContent: helper.PointerOf(bareID),
			validate: func(t *testing.T, dir string, c *Client) {
				must.Eq(t, bareID, c.getID())
			},
		},
		{
			name:        "trims trailing newline",
			fileContent: helper.PointerOf(bareID + "\n"),
			validate: func(t *testing.T, dir string, c *Client) {
				must.Eq(t, bareID, c.getID())
			},
		},
		{
			name:        "trims leading and trailing whitespace",
			fileContent: helper.PointerOf("  " + bareID + "\r\n"),
			validate: func(t *testing.T, dir string, c *Client) {
				must.Eq(t, bareID, c.getID())
			},
		},
		{
			name:        "whitespace-only file generates new ID",
			fileContent: helper.PointerOf("   "),
			validate: func(t *testing.T, dir string, c *Client) {
				id := c.getID()
				must.NotEq(t, "", id)
				must.Eq(t, id, strings.TrimSpace(id))
			},
		},
		{
			name:        "stable across restarts",
			fileContent: nil,
			validate: func(t *testing.T, dir string, c *Client) {
				firstID := c.getID()
				must.NotEq(t, "", firstID)

				// Simulate a restart: new Client, same data directory.
				c2 := newTestClient(t, dir)
				must.NoError(t, c2.generateID())
				must.Eq(t, firstID, c2.getID())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			if tc.fileContent != nil {
				must.NoError(
					t,
					os.WriteFile(filepath.Join(dir, clientIDFileName), []byte(*tc.fileContent), 0600),
				)
			}

			c := newTestClient(t, dir)
			must.NoError(t, c.generateID())

			tc.validate(t, dir, c)
		})
	}
}
