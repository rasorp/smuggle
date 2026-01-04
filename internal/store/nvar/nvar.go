package nvar

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/hashicorp/nomad/api"

	"github.com/rasorp/smuggle/internal/types"
)

type NomadVariableStore struct {
	client     *api.Client
	configPath string
	clientPath string
}

// New creates a new NomadVariableStore with the given Nomad API client and base path.
// The path parameter specifies the base path under which all variables will be stored.
func New(client *api.Client, basePath string) *NomadVariableStore {
	return &NomadVariableStore{
		client:     client,
		configPath: filepath.Join(basePath, "networks", types.StoreVersionLatest),
		clientPath: filepath.Join(basePath, "subnets", types.StoreVersionLatest),
	}
}

// ListNetworks retrieves all the network configurations stored as Nomad
// variables under the configured base path. Each variable is expected to have a
// single item containing the JSON-encoded network configuration as an item
// named "data".
func (s *NomadVariableStore) ListNetworks(
	_ *types.StoreGetNetworksReq,
) (*types.StoreGetNetworksResp, error) {

	varList, _, err := s.client.Variables().List(&api.QueryOptions{Prefix: s.configPath})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	resp := types.StoreGetNetworksResp{}

	for _, varMD := range varList {
		variable, _, err := s.client.Variables().Read(varMD.Path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to read network: %w", err)
		}

		network, err := parseNetwork(variable.Items)
		if err != nil {
			return nil, fmt.Errorf("failed to parse network: %w", err)
		}

		resp.Networks = append(resp.Networks, network)
	}

	return &resp, nil
}

func (s *NomadVariableStore) ListSubnets(
	req *types.StoreListSubnetsReq,
) (*types.StoreListSubnetsResp, error) {

	varList, _, err := s.client.Variables().List(
		&api.QueryOptions{
			Prefix: path.Join(s.clientPath, req.Network),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list subnets: %w", err)
	}

	resp := &types.StoreListSubnetsResp{}

	for _, varStub := range varList {
		variable, _, err := s.client.Variables().Read(varStub.Path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to read subnet: %w", err)
		}

		clientSubnet, err := parseClientSubnetConfig(variable.Items)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet: %w", err)
		}

		resp.Subnets = append(resp.Subnets, clientSubnet)
	}

	return resp, nil
}

func (s *NomadVariableStore) DeleteSubnet(
	req *types.StoreDeleteSubnetReq,
) (*types.StoreDeleteSubnetResp, error) {

	path := filepath.Join(s.clientPath, req.NetworkName, req.ID)

	_, err := s.client.Variables().Delete(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to delete subnet: %w", err)
	}

	return &types.StoreDeleteSubnetResp{}, nil
}

// SetClientConfig stores the client configuration as a Nomad variable.
// The configuration is stored at a path derived from the client's IP address.
func (s *NomadVariableStore) SetSubnet(
	req *types.StoreSetSubnetReq,
) (*types.StoreSetSubnetResp, error) {

	// Convert the client config to JSON and store it in the variable items
	configData, err := json.Marshal(req.Subnet)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal subnet: %w", err)
	}

	variable := &api.Variable{
		Path: path.Join(s.clientPath, req.Subnet.NetworkName, req.Subnet.ClientID),
		Items: map[string]string{
			"data": string(configData),
		},
	}

	// Write the variable
	_, _, err = s.client.Variables().Update(variable, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to write subnet: %w", err)
	}

	return &types.StoreSetSubnetResp{}, nil
}

// parseNetwork converts a Nomad variable item string into a Network
// configuration object.
func parseNetwork(items map[string]string) (*types.Network, error) {

	configJSON, ok := items["data"]
	if !ok {
		return nil, errors.New("data key not found in variable items")
	}

	var subnetConfig types.Network

	if err := json.Unmarshal([]byte(configJSON), &subnetConfig); err != nil {
		return nil, err
	}

	return &subnetConfig, nil
}

// WatchClientConfigs watches for changes to client configurations stored in Nomad variables.
// It returns a channel that receives the current list of client configurations whenever
// a change is detected. The watch continues until the context is cancelled.
//
// The function polls Nomad variables using blocking queries with the WaitIndex to efficiently
// detect changes without excessive API calls.
func (s *NomadVariableStore) WatchSubnets(
	req *types.StoreWatchSubnetsReq,
) (*types.StoreWatchSubnetsResp, error) {

	modifyCh := make(chan []*types.Subnet)
	deleteCh := make(chan []*types.Subnet)
	errCh := make(chan error, 1)

	go func() {
		defer close(modifyCh)
		defer close(deleteCh)
		defer close(errCh)

		// Start with index 0 to get initial state
		waitIndex := uint64(0)

		for {
			select {
			case <-req.Context.Done():
				return
			default:
			}

			// Use blocking query with wait index
			queryOpts := &api.QueryOptions{
				Prefix:    filepath.Join(s.clientPath, req.NetworkName),
				WaitIndex: waitIndex,
				WaitTime:  5 * time.Minute,
			}

			// Add context to query options
			queryOpts = queryOpts.WithContext(req.Context)

			// List all client configuration variables
			varList, queryMeta, err := s.client.Variables().List(queryOpts)
			if err != nil {
				select {
				case errCh <- fmt.Errorf("failed to list subnets: %w", err):
				case <-req.Context.Done():
					return
				}
				// Wait before retrying on error
				select {
				case <-time.After(10 * time.Second):
				case <-req.Context.Done():
					return
				}
				continue
			}

			// Check if the index changed (indicating actual changes)
			if queryMeta.LastIndex <= waitIndex {
				// No changes, continue watching
				continue
			}

			// Parse all client configurations
			var (
				modifiedConfigs []*types.Subnet
				expiredConfigs  []*types.Subnet
			)

			for _, varStub := range varList {

				if varStub.ModifyIndex < waitIndex {
					continue
				}

				variable, _, err := s.client.Variables().Read(varStub.Path, nil)
				if err != nil {
					select {
					case errCh <- fmt.Errorf("failed to read subnet: %w", err):
					case <-req.Context.Done():
						return
					}
					continue
				}

				// Parse the client config
				clientConfig, err := parseClientSubnetConfig(variable.Items)
				if err != nil {
					select {
					case errCh <- fmt.Errorf("failed to parse subnet: %w", err):
					case <-req.Context.Done():
						return
					}
					continue
				}

				if clientConfig.Expired {
					expiredConfigs = append(expiredConfigs, clientConfig)
				} else {
					modifiedConfigs = append(modifiedConfigs, clientConfig)
				}
			}

			// Send expired configurations
			if len(expiredConfigs) > 0 {
				select {
				case deleteCh <- expiredConfigs:
				case <-req.Context.Done():
					return
				}
			}

			if len(modifiedConfigs) > 0 {
				select {
				case modifyCh <- modifiedConfigs:
				case <-req.Context.Done():
					return
				}
			}

			// Update wait index for next iteration
			waitIndex = queryMeta.LastIndex
		}
	}()

	return &types.StoreWatchSubnetsResp{
		ModifyCh: modifyCh,
		DeleteCh: deleteCh,
		ErrorCh:  errCh,
	}, nil
}

// GetClientConfigs retrieves all client subnet configurations stored in Nomad variables.
// It lists all variables under the client path and parses them as ClientSubnet configurations.
func (s *NomadVariableStore) GetSubnet(
	req *types.StoreGetSubnetReq,
) (*types.StoreGetSubnetResp, error) {

	path := filepath.Join(s.clientPath, req.NetworkName, req.ID)

	// Read the full variable
	variable, _, err := s.client.Variables().Read(path, nil)
	if err != nil {
		if errors.Is(err, api.ErrVariablePathNotFound) {
			return &types.StoreGetSubnetResp{}, nil
		}
		return nil, fmt.Errorf("failed to read subnet: %w", err)
	}

	// Parse the client subnet config
	clientSubnet, err := parseClientSubnetConfig(variable.Items)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subnet: %w", err)
	}

	return &types.StoreGetSubnetResp{
		Subnet: clientSubnet,
	}, nil
}

// parseClientSubnetConfig converts a Nomad variable's items map into a ClientSubnet.
func parseClientSubnetConfig(items map[string]string) (*types.Subnet, error) {
	// Look for a "data" key in the items
	configJSON, ok := items["data"]
	if !ok {
		return nil, errors.New("data key not found in variable items")
	}

	var clientSubnet types.Subnet
	if err := json.Unmarshal([]byte(configJSON), &clientSubnet); err != nil {
		return nil, err
	}

	return &clientSubnet, nil
}
