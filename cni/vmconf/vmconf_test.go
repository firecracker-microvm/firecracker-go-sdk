// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package vmconf

import (
	"net"
	"testing"

	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"

	"github.com/firecracker-microvm/firecracker-go-sdk/cni/internal"
)

func TestMTUOf(t *testing.T) {
	netNS := internal.MockNetNS{MockPath: "/my/lil/netns"}

	redirectIfName := "veth0"
	redirectMTU := 1337
	redirectMac, err := net.ParseMAC("22:33:44:55:66:77")
	require.NoError(t, err, "failed to get redirect mac")

	tapName := "tap0"
	tapMTU := 1338
	tapMac, err := net.ParseMAC("11:22:33:44:55:66")
	require.NoError(t, err, "failed to get tap mac")

	nlOps := &internal.MockNetlinkOps{
		CreatedTap: &internal.MockLink{
			LinkAttrs: netlink.LinkAttrs{
				Name:         tapName,
				HardwareAddr: tapMac,
				MTU:          tapMTU,
			},
		},
		RedirectIface: &internal.MockLink{
			LinkAttrs: netlink.LinkAttrs{
				Name:         redirectIfName,
				HardwareAddr: redirectMac,
				MTU:          redirectMTU,
			},
		},
	}

	actualMTU, err := mtuOf(tapName, netNS, nlOps)
	require.NoError(t, err, "failed to get mtu")
	assert.Equal(t, tapMTU, actualMTU, "received unexpected tap MTU")
}

func TestIPBootParams(t *testing.T) {
	staticNetworkConf := &StaticNetworkConf{
		TapName:   "taptaptap",
		NetNSPath: "/my/lil/netns",
		VMMacAddr: "00:11:22:33:44:55",
		VMIfName:  "eth0",
		VMMTU:     1337,
		VMIPConfig: &current.IPConfig{
			Address: net.IPNet{
				IP:   net.IPv4(10, 0, 0, 2),
				Mask: net.IPv4Mask(255, 255, 255, 0),
			},
			Gateway: net.IPv4(10, 0, 0, 1),
		},
		VMRoutes: []*types.Route{{
			Dst: net.IPNet{
				IP:   net.IPv4(192, 168, 0, 2),
				Mask: net.IPv4Mask(255, 255, 0, 0),
			},
			GW: net.IPv4(192, 168, 0, 1),
		}},
		VMNameservers:     []string{"1.1.1.1", "8.8.8.8", "1.0.0.1"},
		VMDomain:          "example.com",
		VMSearchDomains:   []string{"look", "here"},
		VMResolverOptions: []string{"choice", "is", "an", "illusion"},
	}

	expectedIPBootParam := "10.0.0.2::10.0.0.1:255.255.255.0::eth0:off:1.1.1.1:8.8.8.8:"
	actualIPBootParam := staticNetworkConf.IPBootParam()
	assert.Equal(t, expectedIPBootParam, actualIPBootParam)
}
