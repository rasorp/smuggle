package types

// StoreVersionLatest defines the latest version identifier of the state store
// schema. This forms a basis for migrations and schema versioning in the future
// if needed.
//
// Backwards compatible changes do not require a version bump. Breaking changes
// require a new version identifier and the store implementations must handle
// migrations from older versions to the latest.
const StoreVersionLatest = "v1"

type StoreDeleteSubnetReq struct {
	ID          string
	NetworkName string
}

type StoreDeleteSubnetResp struct {
	ModifyIndex uint64
}

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

type StoreSetSubnetResp struct {
	ModifyIndex uint64
}

type StoreGetSubnetReq struct {
	ID          string
	NetworkName string
}

type StoreGetSubnetResp struct {
	Subnet *Subnet
}

type StoreWatchSubnetsReq struct {
	NetworkName string
}

type StoreWatchSubnetsResp struct {
	ModifyCh chan []*Subnet
	DeleteCh chan []*Subnet
	ErrorCh  chan error
}
