package nvar

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"

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

		clientSubnet.ModifyIndex = variable.ModifyIndex
		resp.Subnets = append(resp.Subnets, clientSubnet)
	}

	return resp, nil
}

func (s *NomadVariableStore) DeleteSubnet(
	req *types.StoreDeleteSubnetReq,
) (*types.StoreDeleteSubnetResp, error) {

	path := filepath.Join(s.clientPath, req.NetworkName, req.ID)

	meta, err := s.client.Variables().Delete(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to delete subnet: %w", err)
	}

	return &types.StoreDeleteSubnetResp{ModifyIndex: meta.LastIndex}, nil
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
	updatedVar, _, err := s.client.Variables().Update(variable, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to write subnet: %w", err)
	}

	return &types.StoreSetSubnetResp{ModifyIndex: updatedVar.ModifyIndex}, nil
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

	clientSubnet.ModifyIndex = variable.ModifyIndex

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
