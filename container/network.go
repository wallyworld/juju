// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

const (
	// BridgeNetwork will have the container use the network bridge.
	BridgeNetwork = "bridge"
	// PhyscialNetwork will have the container use a specified network device.
	PhysicalNetwork = "physical"
)

// NetworkConfig defines how the container network will be configured.
type NetworkConfig struct {
	NetworkType string
	Device      string
	MTU         int
}

// BridgeNetworkConfig returns a valid NetworkConfig to use the specified
// device as a network bridge for the container.
func BridgeNetworkConfig(device string, MTU int) *NetworkConfig {
	return &NetworkConfig{BridgeNetwork, device, MTU}
}

// PhysicalNetworkConfig returns a valid NetworkConfig to use the specified
// device as the network device for the container.
func PhysicalNetworkConfig(device string, MTU int) *NetworkConfig {
	return &NetworkConfig{PhysicalNetwork, device, MTU}
}
