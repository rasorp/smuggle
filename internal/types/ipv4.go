package types

import (
	"bytes"
	"errors"
	"fmt"
	"net"
)

// IPv4Net represents an IPv4 network in CIDR notation. It stores the network
// address and prefix length efficiently with useful methods for manipulation
// and comparison.
type IPv4Net struct {
	IP   IPv4Addr `json:"ip"`
	Size uint     `json:"size"`
}

func (i *IPv4Net) UnmarshalJSON(data []byte) error {
	if _, val, err := net.ParseCIDR(string(bytes.Trim(data, "\""))); err != nil {
		return err
	} else {
		*i = fromIPNet(val)
		return nil
	}
}

func (i *IPv4Net) MarshalJSON() ([]byte, error) { return fmt.Appendf(nil, `"%s"`, i), nil }

func (i *IPv4Net) ToIPNet() *net.IPNet {
	return &net.IPNet{
		IP:   i.IP.ToNetIP(),
		Mask: net.CIDRMask(int(i.Size), 32),
	}
}

func (i *IPv4Net) String() string {
	return fmt.Sprintf("%s/%d", i.IP.String(), i.Size)
}

// FromIPNet converts a standard library net.IPNet to our IPv4Net
func fromIPNet(n *net.IPNet) IPv4Net {
	prefixLen, _ := n.Mask.Size()
	return IPv4Net{
		fromIP(n.IP),
		uint(prefixLen),
	}
}

func (i *IPv4Net) mask() uint32 { return 0xFFFFFFFF << (32 - i.Size) }

// FromIP converts a net.IP to an IPv4Addr
func fromIP(ip net.IP) IPv4Addr {
	if ipv4 := ip.To4(); ipv4 == nil {
		panic("unexpected address type; expected IPv4")
	} else {
		return fromBytes(ipv4)
	}
}

// fromBytes converts a 4-byte slice to an IPv4Addr
func fromBytes(ip []byte) IPv4Addr {
	return IPv4Addr(uint32(ip[3]) |
		(uint32(ip[2]) << 8) |
		(uint32(ip[1]) << 16) |
		(uint32(ip[0]) << 24))
}

// NextNetwork returns the next adjacent network of the same size
func (i *IPv4Net) NextNetwork() *IPv4Net {
	return &IPv4Net{
		i.IP + (1 << (32 - i.Size)),
		i.Size,
	}
}

// NextAddr returns the network with IP incremented by 1
func (i *IPv4Net) NextAddr() *IPv4Net {
	n := *i
	n.IP += 1
	return &n
}

// Overlap checks if two networks overlap
func (i *IPv4Net) Overlap(other *IPv4Net) bool {
	var mask uint32
	if i.Size < other.Size {
		mask = i.mask()
	} else {
		mask = other.mask()
	}
	return (uint32(i.IP) & mask) == (uint32(other.IP) & mask)
}

// IPv4Addr represents an IPv4 address as a 32-bit unsigned integer.
// This provides efficient storage and manipulation of IP addresses.
type IPv4Addr uint32

func (i *IPv4Addr) MarshalJSON() ([]byte, error) { return fmt.Appendf(nil, `"%s"`, i), nil }

func (i *IPv4Addr) UnmarshalJSON(j []byte) error {
	j = bytes.Trim(j, "\"")
	if val, err := parseIPv4Addr(string(j)); err != nil {
		return err
	} else {
		*i = val
		return nil
	}
}

// EmptyIPv4Addr represents an unset or zero IPv4 address
var EmptyIPv4Addr = IPv4Addr(0)

// parseIPv4Addr parses a string representation of an IPv4 address
func parseIPv4Addr(ip string) (IPv4Addr, error) {
	if parsedIP := net.ParseIP(ip); parsedIP == nil {
		return 0, errors.New("failed to parse IP address")
	} else {
		ipBytes := parsedIP.To4()

		return IPv4Addr(uint32(ipBytes[3]) |
			(uint32(ipBytes[2]) << 8) |
			(uint32(ipBytes[1]) << 16) |
			(uint32(ipBytes[0]) << 24)), nil
	}
}

func (i IPv4Addr) octets() (a, b, c, d byte) {
	a, b, c, d = byte(i>>24), byte(i>>16), byte(i>>8), byte(i)
	return
}

func (i IPv4Addr) ToNetIP() net.IP { return net.IPv4(i.octets()) }

func (i IPv4Addr) String() string { return i.ToNetIP().String() }
