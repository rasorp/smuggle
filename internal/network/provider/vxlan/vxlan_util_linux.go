package vxlan

import (
	"github.com/vishvananda/netlink"
)

// vxlansEqual compares two VXLAN links for equality based on relevant fields.
func vxlansEqual(link1, link2 netlink.Link) bool {
	if link1.Type() != link2.Type() {
		return false
	}

	v1 := link1.(*netlink.Vxlan)
	v2 := link2.(*netlink.Vxlan)

	if v1.VxlanId != v2.VxlanId {
		return false
	}
	if v1.VtepDevIndex > 0 && v2.VtepDevIndex > 0 && v1.VtepDevIndex != v2.VtepDevIndex {
		return false
	}
	if len(v1.SrcAddr) > 0 && len(v2.SrcAddr) > 0 && !v1.SrcAddr.Equal(v2.SrcAddr) {
		return false
	}
	if len(v1.Group) > 0 && len(v2.Group) > 0 && !v1.Group.Equal(v2.Group) {
		return false
	}
	if v1.L2miss != v2.L2miss {
		return false
	}
	if v1.Port > 0 && v2.Port > 0 && v1.Port != v2.Port {
		return false
	}
	if v1.GBP != v2.GBP {
		return false
	}

	return true
}
