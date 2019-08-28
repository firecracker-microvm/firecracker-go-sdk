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

package internal

import (
	"net"
	"testing"

	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterfaceIPs(t *testing.T) {
	vethName := "veth0"
	vmIfaceName := "vm0"

	vethIndex := 0
	vmIndex := 2

	netnsID := "netns"
	vmID := "vmID"

	result := &current.Result{
		CNIVersion: version.Current(),
		Interfaces: []*current.Interface{
			{
				Name:    vethName,
				Sandbox: netnsID,
			},
			{
				Name:    vmIfaceName,
				Sandbox: netnsID,
			},
			{
				Name:    vmIfaceName,
				Sandbox: vmID,
			},
		},
		IPs: []*current.IPConfig{
			{
				Version:   "4",
				Interface: &vethIndex,
				Address: net.IPNet{
					IP:   net.IPv4(10, 0, 0, 2),
					Mask: net.IPv4Mask(255, 255, 255, 0),
				},
				Gateway: net.IPv4(10, 0, 0, 1),
			},
			{
				Version:   "4",
				Interface: &vmIndex,
				Address: net.IPNet{
					IP:   net.IPv4(10, 0, 1, 2),
					Mask: net.IPv4Mask(255, 255, 255, 0),
				},
				Gateway: net.IPv4(10, 0, 1, 1),
			},
			{
				Version:   "4",
				Interface: &vethIndex,
				Address: net.IPNet{
					IP:   net.IPv4(192, 168, 0, 2),
					Mask: net.IPv4Mask(255, 255, 255, 0),
				},
				Gateway: net.IPv4(192, 168, 0, 1),
			},
		},
	}

	actualVMIPs := InterfaceIPs(result, vmIfaceName, netnsID)
	assert.Len(t, actualVMIPs, 0,
		"unexpected number of vm IPs in netns sandbox")

	actualVMIPs = InterfaceIPs(result, vmIfaceName, vmID)
	require.Len(t, actualVMIPs, 1,
		"unexpected number of vm IPs in vmID sandbox")
	assert.Equal(t, result.IPs[1], actualVMIPs[0],
		"unexpected vm IP in vmID sandbox")

	actualVethIPs := InterfaceIPs(result, vethName, netnsID)
	require.Len(t, actualVethIPs, 2,
		"unexpected number of veth IPs in netns sandbox")
	assert.Equal(t, result.IPs[0], actualVethIPs[0],
		"unexpected veth IP in netns sandbox")
	assert.Equal(t, result.IPs[2], actualVethIPs[1],
		"unexpected veth IP in netns sandbox")

	actualVethIPs = InterfaceIPs(result, vethName, vmID)
	assert.Len(t, actualVethIPs, 0,
		"unexpected number of veth IPs in vmID sandbox")
}

func TestFilterBySandbox(t *testing.T) {
	vethName := "veth0"
	vmIfaceName := "vm0"

	netnsID := "netns"
	vmID := "vmID"

	ifaces := []*current.Interface{
		{
			Name:    vethName,
			Sandbox: netnsID,
		},
		{
			Name:    vmIfaceName,
			Sandbox: netnsID,
		},
		{
			Name:    vmIfaceName,
			Sandbox: vmID,
		},
	}

	in, out := FilterBySandbox(netnsID, ifaces...)
	assert.Equal(t, ifaces[:2], in)
	assert.Equal(t, ifaces[2:], out)

	in, out = FilterBySandbox(vmID, ifaces...)
	assert.Equal(t, ifaces[2:], in)
	assert.Equal(t, ifaces[:2], out)
}

func TestIfacesWithName(t *testing.T) {
	vethName := "veth0"
	vmIfaceName := "vm0"

	netnsID := "netns"
	vmID := "vmID"

	ifaces := []*current.Interface{
		{
			Name:    vethName,
			Sandbox: netnsID,
		},
		{
			Name:    vmIfaceName,
			Sandbox: netnsID,
		},
		{
			Name:    vmIfaceName,
			Sandbox: vmID,
		},
	}

	actualIfaces := IfacesWithName(vethName, ifaces...)
	assert.Equal(t, ifaces[:1], actualIfaces)

	actualIfaces = IfacesWithName(vmIfaceName, ifaces...)
	assert.Equal(t, ifaces[1:], actualIfaces)
}
