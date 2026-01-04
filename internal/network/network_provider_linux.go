package network

import (
	"github.com/rasorp/smuggle/internal/network/provider/vxlan"
	"github.com/rasorp/smuggle/internal/types"
)

func (m *Manager) setProviderMap() {
	m.providers = map[string]types.NetworkProvider{
		"vxlan": vxlan.New(m.logger),
	}
}
