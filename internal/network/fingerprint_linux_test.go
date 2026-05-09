//go:build linux

package network

import (
	"net"
	"testing"

	"github.com/shoenig/test/must"
)

func TestGetDefaultInterface(t *testing.T) {
	iface, err := getDefaultInterface()
	must.NoError(t, err)
	must.NotNil(t, iface)

	// The returned interface must have a non-empty name and be up.
	must.NotEq(t, "", iface.Name)
	must.True(t, iface.Flags&net.FlagUp != 0)

	// It must not be the loopback interface.
	must.True(t, iface.Flags&net.FlagLoopback == 0)

	// It must have at least one non-loopback address.
	addrs, err := iface.Addrs()
	must.NoError(t, err)
	must.True(t, len(addrs) > 0)

	hasRoutable := false
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if !ipNet.IP.IsLoopback() {
			hasRoutable = true
			break
		}
	}
	must.True(t, hasRoutable)
}
