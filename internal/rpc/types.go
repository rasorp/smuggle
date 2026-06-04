package rpc

import (
	"time"

	"github.com/rasorp/smuggle/internal/types"
)

const (
	// NetworkService is the name used when registering the Network RPC handler.
	NetworkService = "Network"

	// SubnetService is the name used when registering the Subnet RPC handler.
	SubnetService = "Subnet"

	// defaultWatchMaxWait is the maximum duration a Watch call will block on
	// the server before returning an empty response. The client immediately
	// issues a follow-up call, so this acts as a ceiling on how long a watcher
	// can be held open without a change.
	defaultWatchMaxWait = 5 * time.Minute
)

// SubnetWatchArgs are the arguments for the SubnetHandler.Watch RPC call.
type SubnetWatchArgs struct {
	NetworkName string
	WaitIndex   uint64
	MaxWait     time.Duration
}

// SubnetWatchReply is the response for a SubnetHandler.Watch RPC call.
type SubnetWatchReply struct {
	// Index is the ModifyIndex of the most recent durable write reflected in
	// this response. Clients must store this value and pass it as WaitIndex on
	// the next call to avoid being unblocked spuriously.
	Index uint64

	// Subnets is the full current subnet snapshot for the requested network.
	Subnets []*types.Subnet
}

// NetworkListArgs are the arguments for the NetworkHandler.List RPC call.
type NetworkListArgs struct{}

// NetworkListReply is the response for a NetworkHandler.List RPC call.
type NetworkListReply struct {
	Networks []*types.Network
}

// SubnetListArgs are the arguments for the SubnetHandler.List RPC call.
type SubnetListArgs struct {
	NetworkName string
}

// SubnetListReply is the response for a SubnetHandler.List RPC call.
type SubnetListReply struct {
	Subnets []*types.Subnet
}

// SubnetGetArgs are the arguments for the SubnetHandler.Get RPC call.
type SubnetGetArgs struct {
	ID          string
	NetworkName string
}

// SubnetGetReply is the response for a SubnetHandler.Get RPC call.
type SubnetGetReply struct {
	Subnet *types.Subnet
}

// SubnetSetArgs are the arguments for the SubnetHandler.Set RPC call.
type SubnetSetArgs struct {
	Subnet *types.Subnet
}

// SubnetSetReply is the response for a SubnetHandler.Set RPC call.
type SubnetSetReply struct{}

// SubnetDeleteArgs are the arguments for the SubnetHandler.Delete RPC call.
type SubnetDeleteArgs struct {
	ID          string
	NetworkName string
}

// SubnetDeleteReply is the response for a SubnetHandler.Delete RPC call.
type SubnetDeleteReply struct{}
