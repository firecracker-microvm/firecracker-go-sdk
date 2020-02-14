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

const (
	vmID = "this-is-not-a-machine"

	redirectInterfaceName = "veth0"
	redirectMTU           = 1337
	redirectMacStr        = "22:33:44:55:66:77"

	tapName   = "tap0"
	tapUID    = 123
	tapGID    = 456
	tapMacStr = "11:22:33:44:55:66"
)

var (
	netNS = internal.MockNetNS{MockPath: "/my/lil/netns"}

	redirectMac net.HardwareAddr
	tapMac      net.HardwareAddr
)

func init() {
	var err error
	redirectMac, err = net.ParseMAC(redirectMacStr)
	if err != nil {
		panic(err.Error())
	}

	tapMac, err = net.ParseMAC(tapMacStr)
	if err != nil {
		panic(err.Error())
	}
}

func defaultTestPlugin() *plugin {
	redirectIfacesIndex := 0

	return &plugin{
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

		currentResult: &current.Result{
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
		},
	}
}

func TestAdd(t *testing.T) {
	testPlugin := defaultTestPlugin()
	origRedirectIface := testPlugin.currentResult.Interfaces[0]
	origRedirectIP := testPlugin.currentResult.IPs[0]

	err := testPlugin.add()
	require.NoError(t, err,
		"failed to add tap device")
	newResult := testPlugin.currentResult

	require.Len(t, newResult.Interfaces,
		3, "adding tap device should increase CNI result interfaces by 2")

	assert.Equal(t, origRedirectIface, newResult.Interfaces[0],
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
	assert.Equal(t, newResult.IPs[0], origRedirectIP,
		"adding tap device should not modify original redirect IP")
}

func TestAddFailsQdiscErr(t *testing.T) {
	testPlugin := defaultTestPlugin()
	nlOps := testPlugin.NetlinkOps.(*internal.MockNetlinkOps)

	nlOps.AddIngressQdiscErr = errors.New("a terrible mistake")
	err := testPlugin.add()
	require.Error(t, err,
		"tap device add should return an error on AddIngressQdisc failure")
	assert.Contains(t, err.Error(), nlOps.AddIngressQdiscErr.Error())
	assert.Len(t, testPlugin.currentResult.Interfaces, 1,
		"tap device add should not append tap interface to results on error")
}

func TestAddFailsRedirectErr(t *testing.T) {
	testPlugin := defaultTestPlugin()
	nlOps := testPlugin.NetlinkOps.(*internal.MockNetlinkOps)

	nlOps.AddRedirectFilterErr = errors.New("a grave error")
	err := testPlugin.add()
	require.Error(t, err,
		"tap device add should return an error on AddRedirectFilter failure")
	assert.Contains(t, err.Error(), nlOps.AddRedirectFilterErr.Error())
	assert.Len(t, testPlugin.currentResult.Interfaces, 1,
		"tap device add should not append tap interface to results on error")
}

func TestAddFailsCreateTapErr(t *testing.T) {
	testPlugin := defaultTestPlugin()
	nlOps := testPlugin.NetlinkOps.(*internal.MockNetlinkOps)

	nlOps.CreateTapErr = errors.New("a bit of a snafu")
	err := testPlugin.add()
	require.Error(t, err,
		"tap device add should return an error on CreateTap failure")
	assert.Contains(t, err.Error(), nlOps.CreateTapErr.Error())
	assert.Len(t, testPlugin.currentResult.Interfaces, 1,
		"tap device add should not append tap interface to results on error")
}

func TestGetCurrentResult(t *testing.T) {
	expectedResult := defaultTestPlugin().currentResult

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

func TestDel(t *testing.T) {
	testPlugin := defaultTestPlugin()
	mockOps := testPlugin.NetlinkOps.(*internal.MockNetlinkOps)

	err := testPlugin.add()
	require.NoError(t, err, "failed to add")

	err = testPlugin.del()
	require.NoError(t, err, "failed to del")

	require.Len(t, mockOps.RemoveIngressQdiscCalls, 1)
	assert.Equal(t, mockOps.RemoveIngressQdiscCalls[0], mockOps.RedirectIface)

	require.Len(t, mockOps.RemoveLinkCalls, 1)
	assert.Equal(t, mockOps.RemoveLinkCalls[0], tapName)
}

func TestDelLinksGone(t *testing.T) {
	mockOps := &internal.MockNetlinkOps{
		CreatedTap: &internal.MockLink{
			LinkAttrs: netlink.LinkAttrs{
				Name: "random-name",
			},
		},
		RedirectIface: &internal.MockLink{
			LinkAttrs: netlink.LinkAttrs{
				Name: "another-random-name",
			},
		},
	}

	testPlugin := &plugin{
		NetlinkOps:            mockOps,
		vmID:                  vmID,
		tapName:               tapName,
		redirectInterfaceName: redirectInterfaceName,
		netNS:                 netNS,
		currentResult: &current.Result{
			CNIVersion: version.Current(),
			Interfaces: []*current.Interface{
				{
					Name:    redirectInterfaceName,
					Sandbox: netNS.Path(),
					Mac:     redirectMac.String(),
				},
				{
					Name:    tapName,
					Sandbox: netNS.Path(),
					Mac:     tapMac.String(),
				},
				{
					Name:    tapName,
					Sandbox: vmID,
					Mac:     redirectMac.String(),
				},
			},
		},
	}

	err := testPlugin.del()
	require.NoError(t, err, "failed to del")

	assert.Len(t, mockOps.RemoveLinkCalls, 1)
	assert.Equal(t, mockOps.RemoveLinkCalls[0], tapName,
		"del should attempt to delete tap even if redirect was not found")
}

func TestDelFailsQdiscErr(t *testing.T) {
	testPlugin := defaultTestPlugin()
	nlOps := testPlugin.NetlinkOps.(*internal.MockNetlinkOps)

	err := testPlugin.add()
	require.NoError(t, err, "failed to add")

	nlOps.RemoveIngressQdiscErr = errors.New("a terrible mistake")
	err = testPlugin.del()
	require.Error(t, err,
		"del should return an error on RemoveIngressQdisc failure")
	assert.Contains(t, err.Error(), nlOps.RemoveIngressQdiscErr.Error())
}

func TestDelFailsRemoveLinkErr(t *testing.T) {
	testPlugin := defaultTestPlugin()
	nlOps := testPlugin.NetlinkOps.(*internal.MockNetlinkOps)

	err := testPlugin.add()
	require.NoError(t, err, "failed to add")

	nlOps.RemoveLinkErr = errors.New("a grave error")
	err = testPlugin.del()
	require.Error(t, err,
		"del should return an error on RemoveLink failure")
	assert.Contains(t, err.Error(), nlOps.RemoveLinkErr.Error())
}

func TestDelFailsGetLinkErr(t *testing.T) {
	testPlugin := defaultTestPlugin()
	nlOps := testPlugin.NetlinkOps.(*internal.MockNetlinkOps)

	err := testPlugin.add()
	require.NoError(t, err, "failed to add")

	nlOps.GetLinkErr = errors.New("a bit of a snafu")
	err = testPlugin.del()
	require.Error(t, err,
		"del should return an error on GetLink failure")
	assert.Contains(t, err.Error(), nlOps.GetLinkErr.Error())
}

func TestCheck(t *testing.T) {
	testPlugin := defaultTestPlugin()

	err := testPlugin.add()
	require.NoError(t, err, "failed to add")

	err = testPlugin.check()
	require.NoError(t, err, "failed to check")
}

func TestCheckFails(t *testing.T) {
	testPlugin := defaultTestPlugin()

	err := testPlugin.check()
	require.Error(t, err, "check should fail when configuration not as expected")
}

func TestNewPlugin(t *testing.T) {
	expectedResult := defaultTestPlugin().currentResult
	nspath := "/tmp/IDoNotExist"
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
	testArgs := skel.CmdArgs{
		ContainerID: "continer-id",
		Netns:       nspath,
		IfName:      "test-name",
		Args:        "TC_REDIRECT_TAP_NAME=tap_name;TC_REDIRECT_TAP_UID=123;TC_REDIRECT_TAP_GID=321",
		Path:        "",
		StdinData:   rawPrevResultBytes,
	}

	plugin, err := newPlugin(&testArgs)
	require.NoError(t, err, "failed to create new plugin")
	assert.Equal(t, plugin.tapName, "tap_name",
		"TC_REDIRECT_TAP_NAME should be equal to `tap_name`")
	assert.Equal(t, plugin.tapGID, 321,
		"TC_REDIRECT_TAP_NAME should be equal to `321`")
	assert.Equal(t, plugin.tapUID, 123,
		"TC_REDIRECT_TAP_NAME should be equal to `123`")
}

func TestExtractArgs(t *testing.T) {
	cliArgs := "key1=val1;key2=val2"
	parsedArgs, err := extractArgs(cliArgs)
	require.NoError(t, err,
		"failed to extract cli args")
	assert.Equal(t, "val2", parsedArgs["key2"],
		"Parameter value for key2 should be `val2`")
}
