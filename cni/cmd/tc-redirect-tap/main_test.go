// Copyright 2018-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package main

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"

	"github.com/firecracker-microvm/firecracker-go-sdk/cni/internal"
)

func TestAdd(t *testing.T) {
	vmID := "this-is-not-a-machine"

	redirectInterfaceName := "veth0"
	redirectMTU := 1337

	redirectMac, err := net.ParseMAC("22:33:44:55:66:77")
	require.NoError(t, err, "failed to get redirect mac")

	tapName := "tap0"
	tapMac, err := net.ParseMAC("11:22:33:44:55:66")
	require.NoError(t, err, "failed to get tap mac")

	tapUID := 123
	tapGID := 456

	netNS := internal.MockNetNS{MockPath: "/my/lil/netns"}

	p := &plugin{
		NetlinkOps: &internal.MockNetlinkOps{
			CreatedTap: &internal.MockLink{
				LinkAttrs: netlink.LinkAttrs{
					Name:         tapName,
					HardwareAddr: tapMac,
				},
			},
			RedirectIface: &internal.MockLink{
				LinkAttrs: netlink.LinkAttrs{
					Name:         redirectInterfaceName,
					HardwareAddr: redirectMac,
					MTU:          redirectMTU,
				},
			},
		},
		vmID:                  vmID,
		tapName:               tapName,
		tapUID:                tapUID,
		tapGID:                tapGID,
		redirectInterfaceName: redirectInterfaceName,
		netNS:                 netNS,
	}

	redirectIfacesIndex := 0

	baseResult := &current.Result{
		CNIVersion: version.Current(),
		Interfaces: []*current.Interface{{
			Name:    "veth0",
			Sandbox: netNS.Path(),
			Mac:     redirectMac.String(),
		}},
		IPs: []*current.IPConfig{{
			Version:   "4",
			Interface: &redirectIfacesIndex,
			Address: net.IPNet{
				IP:   net.IPv4(10, 0, 0, 2),
				Mask: net.IPv4Mask(255, 255, 255, 0),
			},
			Gateway: net.IPv4(10, 0, 0, 1),
		}},
	}

	newResult := &current.Result{
		CNIVersion: baseResult.CNIVersion,
		Interfaces: append([]*current.Interface{}, baseResult.Interfaces...),
		IPs:        append([]*current.IPConfig{}, baseResult.IPs...),
	}

	err = p.add(newResult)
	require.NoError(t, err,
		"failed to add tap device")

	require.Len(t, newResult.Interfaces,
		3, "adding tap device should increase CNI result interfaces by 2")
	assert.Equal(t, baseResult.Interfaces[0], newResult.Interfaces[0],
		"adding tap device should not modify the original redirect interface")

	actualTapIface := newResult.Interfaces[1]
	assert.Equal(t, tapName, actualTapIface.Name,
		"tap device in result should have expected name")
	assert.Equal(t, netNS.Path(), actualTapIface.Sandbox,
		"tap device in result should have expected netns")
	assert.Equal(t, tapMac.String(), actualTapIface.Mac,
		"tap device in result should have expected mac addr")

	actualVMIface := newResult.Interfaces[2]
	assert.Equal(t, tapName, actualVMIface.Name,
		"vm iface in result should have expected name")
	assert.Equal(t, vmID, actualVMIface.Sandbox,
		"vm iface in result should have expected netns")
	assert.Equal(t, redirectMac.String(), actualVMIface.Mac,
		"vm iface in result should have expected mac addr")

	require.Len(t, newResult.IPs, 2,
		"adding tap device should increase CNI result IPs by 1")
	assert.Equal(t, newResult.IPs[0], baseResult.IPs[0],
		"adding tap device should not modify original redirect IP")
}

func TestAddFails(t *testing.T) {
	vmID := "this-is-not-a-machine"

	redirectInterfaceName := "veth0"
	redirectMTU := 1337

	redirectMac, err := net.ParseMAC("22:33:44:55:66:77")
	require.NoError(t, err, "failed to get redirect mac")

	tapName := "tap0"
	tapMac, err := net.ParseMAC("11:22:33:44:55:66")
	require.NoError(t, err, "failed to get tap mac")

	tapUID := 123
	tapGID := 456

	netNS := internal.MockNetNS{MockPath: "/my/lil/netns"}

	nlOps := internal.MockNetlinkOps{
		CreatedTap: &internal.MockLink{
			LinkAttrs: netlink.LinkAttrs{
				Name:         tapName,
				HardwareAddr: tapMac,
			},
		},
		RedirectIface: &internal.MockLink{
			LinkAttrs: netlink.LinkAttrs{
				Name:         redirectInterfaceName,
				HardwareAddr: redirectMac,
				MTU:          redirectMTU,
			},
		},
	}

	p := &plugin{
		NetlinkOps:            &nlOps,
		vmID:                  vmID,
		tapName:               tapName,
		tapUID:                tapUID,
		tapGID:                tapGID,
		redirectInterfaceName: redirectInterfaceName,
		netNS:                 netNS,
	}

	redirectIfacesIndex := 0

	baseResult := &current.Result{
		CNIVersion: version.Current(),
		Interfaces: []*current.Interface{{
			Name:    "veth0",
			Sandbox: netNS.Path(),
			Mac:     redirectMac.String(),
		}},
		IPs: []*current.IPConfig{{
			Version:   "4",
			Interface: &redirectIfacesIndex,
			Address: net.IPNet{
				IP:   net.IPv4(10, 0, 0, 2),
				Mask: net.IPv4Mask(255, 255, 255, 0),
			},
			Gateway: net.IPv4(10, 0, 0, 1),
		}},
	}

	nlOps.AddIngressQdiscErr = errors.New("a terrible mistake")
	result := &current.Result{
		CNIVersion: baseResult.CNIVersion,
		Interfaces: append([]*current.Interface{}, baseResult.Interfaces...),
		IPs:        append([]*current.IPConfig{}, baseResult.IPs...),
	}
	err = p.add(result)
	require.Error(t, err,
		"tap device add should return an error on AddIngressQdisc failure")
	assert.Contains(t, err.Error(), nlOps.AddIngressQdiscErr.Error())
	assert.Len(t, result.Interfaces, 1,
		"tap device add should not append tap interface to results on error")
	nlOps.AddIngressQdiscErr = nil

	nlOps.AddRedirectFilterErr = errors.New("a grave error")
	result = &current.Result{
		CNIVersion: baseResult.CNIVersion,
		Interfaces: append([]*current.Interface{}, baseResult.Interfaces...),
		IPs:        append([]*current.IPConfig{}, baseResult.IPs...),
	}
	err = p.add(result)
	require.Error(t, err,
		"tap device add should return an error on AddRedirectFilter failure")
	assert.Contains(t, err.Error(), nlOps.AddRedirectFilterErr.Error())
	assert.Len(t, result.Interfaces, 1,
		"tap device add should not append tap interface to results on error")
	nlOps.AddRedirectFilterErr = nil

	nlOps.CreateTapErr = errors.New("a bit of a snafu")
	result = &current.Result{
		CNIVersion: baseResult.CNIVersion,
		Interfaces: append([]*current.Interface{}, baseResult.Interfaces...),
		IPs:        append([]*current.IPConfig{}, baseResult.IPs...),
	}
	err = p.add(result)
	require.Error(t, err,
		"tap device add should return an error on CreateTap failure")
	assert.Contains(t, err.Error(), nlOps.CreateTapErr.Error())
	assert.Len(t, result.Interfaces, 1,
		"tap device add should not append tap interface to results on error")
}

func TestGetCurrentResult(t *testing.T) {
	netNS := internal.MockNetNS{MockPath: "/my/lil/netns"}

	redirectMac, err := net.ParseMAC("22:33:44:55:66:77")
	require.NoError(t, err, "failed to get redirect mac")

	redirectIfacesIndex := 0

	expectedResult := &current.Result{
		CNIVersion: version.Current(),
		Interfaces: []*current.Interface{{
			Name:    "veth0",
			Sandbox: netNS.Path(),
			Mac:     redirectMac.String(),
		}},
		IPs: []*current.IPConfig{{
			Version:   "4",
			Interface: &redirectIfacesIndex,
			Address: net.IPNet{
				IP:   net.IPv4(10, 0, 0, 2),
				Mask: net.IPv4Mask(255, 255, 255, 0),
			},
			Gateway: net.IPv4(10, 0, 0, 1),
		}},
		Routes: []*types.Route{},
		DNS: types.DNS{
			Nameservers: []string{"1.1.1.1", "8.8.8.8"},
			Domain:      "example.com",
			Search:      []string{"look", "here"},
			Options:     []string{"choice", "is", "an", "illusion"},
		},
	}

	netConf := &types.NetConf{
		CNIVersion: "0.3.1",
		Name:       "my-lil-network",
		Type:       "my-lil-plugin",
		RawPrevResult: map[string]interface{}{
			"cniVersion": "0.3.1",
			"interfaces": expectedResult.Interfaces,
			"ips":        expectedResult.IPs,
			"routes":     expectedResult.Routes,
			"dns":        expectedResult.DNS,
		},
	}

	rawPrevResultBytes, err := json.Marshal(netConf)
	require.NoError(t, err, "failed to marshal mock net conf")

	cmdArgs := &skel.CmdArgs{
		StdinData: rawPrevResultBytes,
	}

	actualResult, err := getCurrentResult(cmdArgs)
	require.NoError(t, err, "failed to get current result from mock net conf")

	assert.Equal(t, expectedResult, actualResult)
}
