//go:build !linux

package network

import (
	"testing"

	"github.com/shoenig/test/must"
	"go.uber.org/zap"
)

func TestManager_setProviderMap(t *testing.T) {
	m := &Manager{logger: zap.NewNop(), dataDir: t.TempDir()}
	m.setProviderMap()
	must.MapLen(t, 0, m.providers)
}
