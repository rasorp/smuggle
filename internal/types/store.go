package types

import (
	"context"
)

// StoreVersionLatest defines the latest version identifier of the state store
// schema. This forms a basis for migrations and schema versioning in the future
// if needed.
//
// Backwards compatible changes do not require a version bump. Breaking changes
// require a new version identifier and the store implementations must handle
// migrations from older versions to the latest.
const StoreVersionLatest = "v1"

// Store defines the interface for persisting and retrieving network and subnet
// configurations. Each implementation is responsible for managing the storage
// and how version schema migrations are handled as well as their own internal
// data structures.
type Store interface {
	ListNetworks(*StoreGetNetworksReq) (*StoreGetNetworksResp, error)

	ListSubnets(*StoreListSubnetsReq) (*StoreListSubnetsResp, error)

	DeleteSubnet(*StoreDeleteSubnetReq) (*StoreDeleteSubnetResp, error)

	GetSubnet(*StoreGetSubnetReq) (*StoreGetSubnetResp, error)

	SetSubnet(*StoreSetSubnetReq) (*StoreSetSubnetResp, error)

	WatchSubnets(*StoreWatchSubnetsReq) (*StoreWatchSubnetsResp, error)
}

type StoreDeleteSubnetReq struct {
	ID          string
	NetworkName string
}

type StoreDeleteSubnetResp struct{}

type StoreGetNetworksReq struct{}

type StoreGetNetworksResp struct {
	Networks []*Network
}

type StoreListSubnetsReq struct {
	Network string
}

type StoreListSubnetsResp struct {
	Subnets []*Subnet
}

type StoreSetSubnetReq struct {
	Subnet *Subnet
}

type StoreSetSubnetResp struct{}

type StoreGetSubnetReq struct {
	ID          string
	NetworkName string
}

type StoreGetSubnetResp struct {
	Subnet *Subnet
}

type StoreWatchSubnetsReq struct {
	NetworkName string
	Context     context.Context
}

type StoreWatchSubnetsResp struct {
	ModifyCh chan []*Subnet
	DeleteCh chan []*Subnet
	ErrorCh  chan error
}
