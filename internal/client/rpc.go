package client

import (
	"context"

	"github.com/rasorp/smuggle/internal/types"
)

// Server is the interface through which the client communicates with a remote
// Smuggle server to read and write subnet state and watch for changes.
type Server interface {
	ListNetworks(*types.StoreGetNetworksReq) (*types.StoreGetNetworksResp, error)
	ListSubnets(*types.StoreListSubnetsReq) (*types.StoreListSubnetsResp, error)
	GetSubnet(*types.StoreGetSubnetReq) (*types.StoreGetSubnetResp, error)
	SetSubnet(*types.StoreSetSubnetReq) (*types.StoreSetSubnetResp, error)
	WatchSubnets(context.Context, *types.StoreWatchSubnetsReq) (*types.StoreWatchSubnetsResp, error)
}
