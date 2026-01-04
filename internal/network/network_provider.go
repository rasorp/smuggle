//go:build !linux

package network

import "github.com/rasorp/smuggle/internal/types"

func (m *Manager) setProviderMap() { m.providers = map[string]types.NetworkProvider{} }
