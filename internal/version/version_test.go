package version

import (
	"testing"

	"github.com/shoenig/test/must"
)

func Test_Get(t *testing.T) {
	testCases := []struct {
		name              string
		version           string
		versionPrerelease string
		expected          string
	}{
		{
			name:              "final release with no prerelease",
			version:           "1.0.0",
			versionPrerelease: "",
			expected:          "1.0.0",
		},
		{
			name:              "release with alpha prerelease",
			version:           "0.0.1",
			versionPrerelease: "alpha.1",
			expected:          "0.0.1-alpha.1",
		},
		{
			name:              "release with beta prerelease",
			version:           "1.2.3",
			versionPrerelease: "beta.2",
			expected:          "1.2.3-beta.2",
		},
		{
			name:              "release with rc prerelease",
			version:           "2.0.0",
			versionPrerelease: "rc1",
			expected:          "2.0.0-rc1",
		},
		{
			name:              "release with dev prerelease",
			version:           "1.5.0",
			versionPrerelease: "dev",
			expected:          "1.5.0-dev",
		},
		{
			name:              "release with complex prerelease",
			version:           "3.1.4",
			versionPrerelease: "alpha.3+build.123",
			expected:          "3.1.4-alpha.3+build.123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			origVersion := version
			origVersionPrerelease := versionPrerelease

			t.Cleanup(
				func() {
					version = origVersion
					versionPrerelease = origVersionPrerelease
				},
			)

			// Set the variables to the test case values and perform our test.
			version = tc.version
			versionPrerelease = tc.versionPrerelease
			must.Eq(t, tc.expected, Get())
		})
	}
}
