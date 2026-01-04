package types

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestSubnet_InterfaceName(t *testing.T) {
	subnet := &Subnet{NetworkName: "vxlan"}
	must.Eq(t, "vxlan0", subnet.InterfaceName())
}
