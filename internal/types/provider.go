package types

import (
	"net"
)

// NetworkProvider defines the interface for network provider implementations.
// Providers are responsible for setting up the underlying network infrastructure
// (e.g., VXLAN, WireGuard, etc.) for the Smuggle network fabric.
type NetworkProvider interface {
	// Name returns the unique identifier for this provider.
	Name() string

	// SetLocal configures the local subnet for this host.
	SetLocal(*NetworkProviderSetReq) (*NetworkProviderSetResp, error)

	// DeleteRemote removes a remote subnet's routing configuration.
	DeleteRemote(*NetworkProviderDeleteRemoteReq) (*NetworkProviderDeleteRemoteResp, error)

	// SetRemote configures routing to a remote subnet.
	SetRemote(*NetworkProviderSetRemoteReq) (*NetworkProviderSetRemoteResp, error)
}

// NetworkProviderSetReq contains parameters for setting up a local subnet.
type NetworkProviderSetReq struct {
	HostInteface *net.Interface
	Client       *Subnet
}

// NetworkProviderSetResp contains the result of setting up a local subnet.
type NetworkProviderSetResp struct {
	Network *Subnet
}

// NetworkProviderDeleteRemoteReq contains parameters for deleting a remote subnet.
type NetworkProviderDeleteRemoteReq struct {
	Subnet *Subnet
}

// NetworkProviderDeleteRemoteResp is returned after deleting a remote subnet.
type NetworkProviderDeleteRemoteResp struct{}

// NetworkProviderSetRemoteReq contains parameters for setting up a remote subnet.
type NetworkProviderSetRemoteReq struct {
	Subnet *Subnet
}

// NetworkProviderSetRemoteResp is returned after setting up a remote subnet.
type NetworkProviderSetRemoteResp struct{}
