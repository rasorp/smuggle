package network

import (
	"testing"

	"github.com/shoenig/test/must"
	"go.uber.org/zap"
)

func TestManager_setProviderMap(t *testing.T) {
	m := &Manager{logger: zap.NewNop(), dataDir: t.TempDir()}
	m.setProviderMap()

	// The map must contain exactly the two expected providers — no more, no
	// fewer. A missing entry would cause any request for that provider to
	// return "unknown network provider"; an extra entry would indicate an
	// unintentional registration.
	must.MapLen(t, 2, m.providers)

	for _, tc := range []struct {
		key  string
		name string
	}{
		{key: "vxlan", name: "vxlan"},
		{key: "wireguard", name: "wireguard"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			p, ok := m.providers[tc.key]
			must.True(t, ok)
			must.Eq(t, tc.name, p.Name())
		})
	}
}
