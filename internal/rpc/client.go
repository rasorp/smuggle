package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	netrpc "net/rpc"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

const (
	// defaultDialTimeout is the maximum time allowed to establish a TCP
	// connection to an RPC server.
	defaultDialTimeout = 10 * time.Second

	// defaultWatchRetryDelay is the time the watch loop waits before retrying
	// after a failed RPC call.
	defaultWatchRetryDelay = 10 * time.Second
)

// Client proxies RPC calls to a remote Smuggle server via net/rpc.
// WatchSubnets uses blocking queries: each poll blocks on the server until the
// subnet index changes, then the client diffs the returned snapshot against its
// previous one to derive modify and delete sets.
type Client struct {
	addrs  []string
	logger *zap.Logger

	mu   sync.Mutex
	conn *netrpc.Client

	// nextAddr tracks the round-robin cursor into addrs for reconnects.
	nextAddr int
}

// NewClient creates a Client that connects to one of the given server addresses.
// At least one address is required.
func NewClient(addrs []string, logger *zap.Logger) (*Client, error) {
	if len(addrs) == 0 {
		return nil, errors.New("at least one server address is required")
	}
	c := &Client{addrs: addrs, logger: logger}
	if err := c.connect(nil); err != nil {
		return nil, err
	}
	return c, nil
}

// connect dials the next address in round-robin order and installs a fresh
// net/rpc client. If failed is non-nil, the reconnect is skipped when c.conn
// is no longer the failed connection, meaning another goroutine has already
// reconnected and a new connection is ready to use.
func (c *Client) connect(failed *netrpc.Client) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if failed != nil && c.conn != failed {
		// Another goroutine already reconnected; nothing to do.
		return nil
	}

	addr := c.addrs[c.nextAddr%len(c.addrs)]
	c.nextAddr++

	conn, err := net.DialTimeout("tcp", addr, defaultDialTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect to RPC server: %w", err)
	}

	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = netrpc.NewClient(conn)
	c.logger.Info("connected to RPC server", zap.String("address", addr))
	return nil
}

// isConnectionLost reports whether err indicates a broken or closed
// connection that warrants a reconnect attempt.
func isConnectionLost(err error) bool {
	return errors.Is(err, netrpc.ErrShutdown) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

// call executes a synchronous RPC. On any connection-loss error it reconnects
// once and retries. The mutex is only held long enough to copy the client
// pointer so that concurrent calls (including long-blocking WatchSubnets) do
// not serialize against one another.
func (c *Client) call(method string, args, reply any) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	err := conn.Call(method, args, reply)
	if err == nil {
		return nil
	}
	if !isConnectionLost(err) {
		return err
	}

	// Connection was closed; try once to reconnect, passing the failed
	// connection so a concurrent reconnect by another goroutine is detected.
	if rerr := c.connect(conn); rerr != nil {
		return fmt.Errorf("%w (reconnect failed: %v)", err, rerr)
	}

	c.mu.Lock()
	conn = c.conn
	c.mu.Unlock()
	return conn.Call(method, args, reply)
}

func (c *Client) ListNetworks(_ *types.StoreGetNetworksReq) (*types.StoreGetNetworksResp, error) {
	reply := &NetworkListReply{}
	if err := c.call(NetworkService+".List", &NetworkListArgs{}, reply); err != nil {
		return nil, err
	}
	return &types.StoreGetNetworksResp{Networks: reply.Networks}, nil
}

func (c *Client) ListSubnets(req *types.StoreListSubnetsReq) (*types.StoreListSubnetsResp, error) {
	reply := &SubnetListReply{}
	if err := c.call(SubnetService+".List", &SubnetListArgs{NetworkName: req.Network}, reply); err != nil {
		return nil, err
	}
	return &types.StoreListSubnetsResp{Subnets: reply.Subnets}, nil
}

func (c *Client) GetSubnet(req *types.StoreGetSubnetReq) (*types.StoreGetSubnetResp, error) {
	reply := &SubnetGetReply{}
	args := &SubnetGetArgs{ID: req.ID, NetworkName: req.NetworkName}
	if err := c.call(SubnetService+".Get", args, reply); err != nil {
		return nil, err
	}
	return &types.StoreGetSubnetResp{Subnet: reply.Subnet}, nil
}

func (c *Client) SetSubnet(req *types.StoreSetSubnetReq) (*types.StoreSetSubnetResp, error) {
	reply := &SubnetSetReply{}
	if err := c.call(SubnetService+".Set", &SubnetSetArgs{Subnet: req.Subnet}, reply); err != nil {
		return nil, err
	}
	return &types.StoreSetSubnetResp{}, nil
}

func (c *Client) DeleteSubnet(req *types.StoreDeleteSubnetReq) (*types.StoreDeleteSubnetResp, error) {
	reply := &SubnetDeleteReply{}
	args := &SubnetDeleteArgs{ID: req.ID, NetworkName: req.NetworkName}
	if err := c.call(SubnetService+".Delete", args, reply); err != nil {
		return nil, err
	}
	return &types.StoreDeleteSubnetResp{}, nil
}

// WatchSubnets starts a background blocking-query loop and returns immediately
// with the response channels. The loop polls the server, diffing each snapshot
// against the previous one to send only the subnets that actually changed.
func (c *Client) WatchSubnets(ctx context.Context, req *types.StoreWatchSubnetsReq) (*types.StoreWatchSubnetsResp, error) {
	modifyCh := make(chan []*types.Subnet, 1)
	deleteCh := make(chan []*types.Subnet, 1)
	errCh := make(chan error, 1)

	go c.watchLoop(ctx, req.NetworkName, modifyCh, deleteCh, errCh)

	return &types.StoreWatchSubnetsResp{
		ModifyCh: modifyCh,
		DeleteCh: deleteCh,
		ErrorCh:  errCh,
	}, nil
}

// watchLoop is the long-running blocking-query poll loop for WatchSubnets.
func (c *Client) watchLoop(
	ctx context.Context,
	networkName string,
	modifyCh chan<- []*types.Subnet,
	deleteCh chan<- []*types.Subnet,
	errCh chan<- error,
) {
	var lastIndex uint64
	var lastSnapshot map[string]*types.Subnet // clientID -> Subnet

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		args := &SubnetWatchArgs{
			NetworkName: networkName,
			WaitIndex:   lastIndex,
			MaxWait:     defaultWatchMaxWait,
		}
		reply := &SubnetWatchReply{}

		if err := c.call(SubnetService+".Watch", args, reply); err != nil {
			select {
			case errCh <- fmt.Errorf("WatchSubnets RPC failed: %w", err):
			case <-ctx.Done():
				return
			}
			t := time.NewTimer(defaultWatchRetryDelay)
			select {
			case <-t.C:
				// Timer fired normally; no cleanup needed.
			case <-ctx.Done():
				if !t.Stop() {
					<-t.C
				}
				return
			}
			continue
		}

		modified, deleted := diffSubnets(lastSnapshot, reply.Subnets)

		if len(modified) > 0 {
			select {
			case modifyCh <- modified:
			case <-ctx.Done():
				return
			}
		}
		if len(deleted) > 0 {
			select {
			case deleteCh <- deleted:
			case <-ctx.Done():
				return
			}
		}

		lastIndex = reply.Index

		// Advance the snapshot, retaining only non-expired subnets so future
		// diffs can correctly detect deletions.
		lastSnapshot = make(map[string]*types.Subnet, len(reply.Subnets))
		for _, subnet := range reply.Subnets {
			if !subnet.Expired {
				lastSnapshot[subnet.ClientID] = subnet
			}
		}
	}
}

// diffSubnets computes the changed and deleted sets relative to a previous
// snapshot. Expired subnets and subnets present in prev but absent from curr
// are treated as deleted. New and updated non-expired subnets are modified.
//
// When prev is nil (first call or after a reconnect) every non-expired subnet
// in curr is emitted as modified, so the caller must be idempotent — applying
// a subnet that is already correctly configured must be a safe no-op.
func diffSubnets(prev map[string]*types.Subnet, curr []*types.Subnet) (modified, deleted []*types.Subnet) {
	currMap := make(map[string]*types.Subnet, len(curr))
	for _, s := range curr {
		currMap[s.ClientID] = s
	}

	for _, s := range curr {
		if s.Expired {
			deleted = append(deleted, s)
			continue
		}
		existing, existed := prev[s.ClientID]
		if !existed || !subnetRoutingEqual(existing, s) {
			modified = append(modified, s)
		}
	}

	for id, s := range prev {
		if _, ok := currMap[id]; !ok {
			deleted = append(deleted, s)
		}
	}
	return
}

// subnetRoutingEqual returns true when two subnets have identical fields that
// affect host routing (HostIPv4, IPv4Network, MTU, Provider). Expiration and
// Expired are intentionally excluded: a heartbeat update must not trigger a
// redundant route reconfiguration.
func subnetRoutingEqual(a, b *types.Subnet) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.ClientID != b.ClientID || a.NetworkName != b.NetworkName {
		return false
	}
	if a.Provider != b.Provider || a.MTU != b.MTU {
		return false
	}
	if ipStr(a.HostIPv4) != ipStr(b.HostIPv4) {
		return false
	}
	return netStr(a.IPv4Network) == netStr(b.IPv4Network)
}

func ipStr(ip *net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

func netStr(n *types.IPv4Net) string {
	if n == nil {
		return ""
	}
	return n.String()
}
