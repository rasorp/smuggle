package store

import "github.com/rasorp/smuggle/internal/types"

// BackingStore is the interface for the server-side persistent store. It is
// the only component in Smuggle that communicates directly with the backing
// data service. Clients obtain subnet and network information exclusively
// through the RPC layer.
type BackingStore interface {
	ListNetworks(*types.StoreGetNetworksReq) (*types.StoreGetNetworksResp, error)
	ListSubnets(*types.StoreListSubnetsReq) (*types.StoreListSubnetsResp, error)
	GetSubnet(*types.StoreGetSubnetReq) (*types.StoreGetSubnetResp, error)
	SetSubnet(*types.StoreSetSubnetReq) (*types.StoreSetSubnetResp, error)
	DeleteSubnet(*types.StoreDeleteSubnetReq) (*types.StoreDeleteSubnetResp, error)
}
