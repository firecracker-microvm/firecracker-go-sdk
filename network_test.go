// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package firecracker

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/firecracker-microvm/firecracker-go-sdk/fctesting"
	"github.com/sparrc/go-ping"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	mockMacAddrString = "00:11:22:33:44:55"
	tapName           = "tap0"

	cniNetworkName = "phony-network"
	mockNetNSPath  = "/my/phony/netns"

	kernelArgsNoIP   = parseKernelArgs("foo=bar this=phony")
	kernelArgsWithIP = parseKernelArgs("foo=bar this=phony ip=whatevz")

	// These RFC 5737 IPs are reserved for documentation, they are not usable
	validIPConfiguration = &IPConfiguration{
		IPAddr: net.IPNet{
			IP:   net.IPv4(198, 51, 100, 2),
			Mask: net.IPv4Mask(255, 255, 255, 0),
		},
		Gateway:     net.IPv4(198, 51, 100, 1),
		Nameservers: []string{"192.0.2.1", "192.0.2.2"},
	}

	// IPv6 is currently invalid
	// These RFC 3849 IPs are reserved for documentation, they are not usable
	invalidIPConfiguration = &IPConfiguration{
		IPAddr: net.IPNet{
			IP:   net.ParseIP("2001:db8:a0b:12f0::2"),
			Mask: net.CIDRMask(24, 128),
		},
		Gateway: net.ParseIP("2001:db8:a0b:12f0::1"),
	}

	validStaticNetworkInterface = NetworkInterface{
		StaticConfiguration: &StaticNetworkConfiguration{
			MacAddress:      mockMacAddrString,
			HostDevName:     tapName,
			IPConfiguration: validIPConfiguration,
		},
	}

	validCNIInterface = NetworkInterface{
		CNIConfiguration: &CNIConfiguration{
			NetworkName: cniNetworkName,
			netNSPath:   mockNetNSPath,
		},
	}
)

func TestNetworkStaticValidation(t *testing.T) {
	err := validStaticNetworkInterface.StaticConfiguration.validate()
	assert.NoError(t, err, "valid network config unexpectedly returned validation error")
}

func TestNetworkStaticValidationFails_HostDevName(t *testing.T) {
	staticNetworkConfig := StaticNetworkConfiguration{
		MacAddress:      mockMacAddrString,
		HostDevName:     "",
		IPConfiguration: validIPConfiguration,
	}

	err := staticNetworkConfig.validate()
	assert.Error(t, err, "invalid network config hostdevname did not result in validation error")
}

func TestNetworkStaticValidationFails_TooManyNameservers(t *testing.T) {
	staticNetworkConfig := StaticNetworkConfiguration{
		MacAddress:  mockMacAddrString,
		HostDevName: tapName,
		IPConfiguration: &IPConfiguration{
			IPAddr: net.IPNet{
				IP:   net.IPv4(198, 51, 100, 2),
				Mask: net.IPv4Mask(255, 255, 255, 0),
			},
			Gateway:     net.IPv4(198, 51, 100, 1),
			Nameservers: []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"},
		},
	}

	err := staticNetworkConfig.validate()
	assert.Error(t, err, "network config with more than two nameservers did not result in validation error")
}

func TestNetworkStaticValidationFails_IPConfiguration(t *testing.T) {
	staticNetworkConfig := StaticNetworkConfiguration{
		MacAddress:      mockMacAddrString,
		HostDevName:     tapName,
		IPConfiguration: invalidIPConfiguration,
	}

	err := staticNetworkConfig.validate()
	assert.Error(t, err, "invalid network config hostdevname did not result in validation error")
}

func TestNetworkCNIValidation(t *testing.T) {
	err := validCNIInterface.CNIConfiguration.validate()
	assert.NoError(t, err, "valid cni network config unexpectedly returned validation error")
}

func TestNetworkCNIValidationFails_NetworkName(t *testing.T) {
	err := CNIConfiguration{}.validate()
	assert.Error(t, err, "invalid cni network config networkname did not result in validation error")
}

func TestNetworkInterfacesValidation_None(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{}).validate(kernelArgsNoIP)
	assert.NoError(t, err, "empty network interface list unexpectedly resulted in validation error")
}

func TestNetworkInterfacesValidation_Static(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{
		validStaticNetworkInterface,
	}).validate(kernelArgsNoIP)
	assert.NoError(t, err, "network interface list with one valid static interface unexpectedly resulted in validation error")
}

func TestNetworkInterfacesValidation_CNI(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{
		validCNIInterface,
	}).validate(kernelArgsNoIP)
	assert.NoError(t, err, "network interface list with one valid CNI interface unexpectedly resulted in validation error")
}

func TestNetworkInterfacesValidation_MultipleStatic(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{
		NetworkInterface{
			StaticConfiguration: &StaticNetworkConfiguration{
				MacAddress:  mockMacAddrString,
				HostDevName: tapName,
			},
		},
		NetworkInterface{
			StaticConfiguration: &StaticNetworkConfiguration{
				MacAddress:  "11:22:33:44:55:66",
				HostDevName: "tap1",
			},
		},
	}).validate(kernelArgsNoIP)
	assert.NoError(t, err, "network interface list with multiple static interfaces unexpectedly resulted in validation error")
}

func TestNetworkInterfacesValidationFails_MultipleCNI(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{
		validCNIInterface,
		NetworkInterface{
			CNIConfiguration: &CNIConfiguration{
				NetworkName: "something-else",
				netNSPath:   "/a/different/netns",
			},
		},
	}).validate(kernelArgsNoIP)
	assert.Error(t, err, "network interface list with multiple CNI interfaces should have resulted in validation error")
}

func TestNetworkInterfacesValidationFails_IPWithMultiple(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{
		validStaticNetworkInterface,
		NetworkInterface{
			StaticConfiguration: &StaticNetworkConfiguration{
				MacAddress:  "11:22:33:44:55:66",
				HostDevName: "tap1",
			},
		},
	}).validate(kernelArgsNoIP)
	assert.Error(t, err, "network interface list with multiple interfaces and IP configuration should return validation error")
}

func TestNetworkInterfacesValidationFails_IPWithKernelArg(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{
		validStaticNetworkInterface,
	}).validate(kernelArgsWithIP)
	assert.Error(t, err, "network interface list with static IP config and IP kernel arg should return validation error")
}

func TestNetworkInterfacesValidationFails_CNIWithMultiple(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{
		validCNIInterface,
		NetworkInterface{
			StaticConfiguration: &StaticNetworkConfiguration{
				MacAddress:  "11:22:33:44:55:66",
				HostDevName: "tap1",
			},
		},
	}).validate(kernelArgsNoIP)
	assert.Error(t, err, "network interface list with multiple interfaces and CNI configuration should return validation error")
}

func TestNetworkInterfacesValidationFails_CNIWithKernelArg(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{
		validCNIInterface,
	}).validate(kernelArgsWithIP)
	assert.Error(t, err, "network interface list with CNI config and IP kernel arg should return validation error")
}

func TestNetworkInterfacesValidationFails_NeitherSpecified(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{{}}).validate(kernelArgsNoIP)
	assert.Error(t, err, "invalid network config with neither static nor cni configuration did not result in validation error")
}

func TestNetworkInterfacesValidationFails_BothSpecified(t *testing.T) {
	err := NetworkInterfaces([]NetworkInterface{{
		StaticConfiguration: &StaticNetworkConfiguration{
			MacAddress:  mockMacAddrString,
			HostDevName: tapName,
		},
		CNIConfiguration: validCNIInterface.CNIConfiguration,
	}}).validate(kernelArgsNoIP)
	assert.Error(t, err, "invalid network config with both static and cni configuration did not result in validation error")
}

func TestNetworkMachineCNI(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	fctesting.RequiresRoot(t)

	cniBinPath := []string{"/opt/cni/bin", testDataBin}

	testCNIDir := filepath.Join(testDataPath, "TestCNI")
	os.RemoveAll(testCNIDir)
	defer os.RemoveAll(testCNIDir)

	cniCacheDir := filepath.Join(testCNIDir, "cni.cache")
	require.NoError(t,
		os.MkdirAll(cniCacheDir, 0755),
		"failed to create cni cache dir")

	cniConfDir := filepath.Join(testCNIDir, "cni.conf")
	require.NoError(t,
		os.MkdirAll(cniConfDir, 0755),
		"failed to create cni conf dir")

	const ifName = "veth0"
	const networkName = "fcnet"

	cniConf := fmt.Sprintf(`{
  "cniVersion": "0.3.1",
  "name": "%s",
  "plugins": [
    {
      "type": "ptp",
      "ipam": {
        "type": "host-local",
        "subnet": "10.168.0.0/16",
        "resolvConf": "/etc/resolv.conf"
      }
    },
    {
      "type": "tc-redirect-tap"
    }
  ]
}`, networkName)

	cniConfPath := filepath.Join(cniConfDir, fmt.Sprintf("%s.conflist", networkName))
	require.NoError(t,
		ioutil.WriteFile(cniConfPath, []byte(cniConf), 0644),
		"failed to write cni conf file")

	numVMs := 10
	vmIPs := make(chan string, numVMs)

	// used as part of the VMIDs to make sure different test suites don't use the same CNI ContainerID (which can reak havok)
	timestamp := time.Now().UnixNano()

	var vmWg sync.WaitGroup
	for i := 0; i < numVMs; i++ {
		vmWg.Add(1)
		go func(vmID string) {
			defer vmWg.Done()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			firecrackerSockPath := filepath.Join(testCNIDir, fmt.Sprintf("%s.sock", vmID))
			rootfsPath := filepath.Join(testCNIDir, fmt.Sprintf("%s.img", vmID))
			expectedCacheDirPath := filepath.Join(cniCacheDir, "results",
				fmt.Sprintf("%s-%s-%s", networkName, vmID, ifName))

			m, vmIP := startCNIMachine(t, ctx,
				firecrackerSockPath, rootfsPath, cniConfDir, cniCacheDir, networkName, ifName, vmID, cniBinPath)
			vmIPs <- vmIP

			assert.FileExists(t, expectedCacheDirPath, "CNI cache dir doesn't exist after vm startup")

			testPing(t, vmIP, 3, 5*time.Second)

			require.NoError(t, m.StopVMM(), "failed to stop machine")
			waitCtx, waitCancel := context.WithTimeout(ctx, 3*time.Second)
			assert.NoError(t, m.Wait(waitCtx), "failed waiting for machine stop")
			waitCancel()

			_, err := os.Stat(expectedCacheDirPath)
			assert.True(t, os.IsNotExist(err), "expected CNI cache dir to not exist after vm exit")

		}(fmt.Sprintf("%d-%s-%d", timestamp, networkName, i))
	}
	vmWg.Wait()
	close(vmIPs)

	vmIPSet := make(map[string]bool)
	for vmIP := range vmIPs {
		if _, ok := vmIPSet[vmIP]; ok {
			assert.Failf(t, "unexpected duplicate vm IP %s", vmIP)
		} else {
			vmIPSet[vmIP] = true
		}
	}
}

func startCNIMachine(t *testing.T,
	ctx context.Context,
	firecrackerSockPath,
	rootfsPath,
	cniConfDir,
	cniCacheDir,
	networkName,
	ifName,
	vmID string,
	cniBinPath []string,
) (*Machine, string) {
	rootfsBytes, err := ioutil.ReadFile(testRootfs)
	require.NoError(t, err, "failed to read rootfs file")
	err = ioutil.WriteFile(rootfsPath, rootfsBytes, 0666)
	require.NoError(t, err, "failed to copy vm rootfs to %s", rootfsPath)

	m, err := NewMachine(ctx, Config{
		SocketPath:      firecrackerSockPath,
		KernelImagePath: getVmlinuxPath(t),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(2),
			MemSizeMib: Int64(256),
			HtEnabled:  Bool(true),
		},
		Drives: []models.Drive{
			models.Drive{
				DriveID:      String("1"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(rootfsPath),
			},
		},
		NetworkInterfaces: []NetworkInterface{{
			CNIConfiguration: &CNIConfiguration{
				ConfDir:     cniConfDir,
				BinPath:     cniBinPath,
				CacheDir:    cniCacheDir,
				NetworkName: networkName,
				IfName:      ifName,
			},
		}},
		VMID: vmID,
	})
	require.NoError(t, err, "failed to create machine with CNI network interface")

	err = m.Start(context.Background())
	require.NoError(t, err, "failed to start machine")

	staticConfig := m.Cfg.NetworkInterfaces[0].StaticConfiguration
	require.NotNil(t, staticConfig,
		"cni configuration should have updated network interface static configuration")
	assert.NotEmpty(t, staticConfig.MacAddress,
		"static config should have mac address set")
	assert.NotEmpty(t, staticConfig.HostDevName,
		"static config should have host dev name set")

	ipConfig := staticConfig.IPConfiguration
	require.NotNil(t, ipConfig,
		"cni configuration should have updated network interface ip configuration")

	return m, ipConfig.IPAddr.IP.String()
}

func testPing(t *testing.T, ip string, count int, timeout time.Duration) {
	pinger, err := ping.NewPinger(ip)
	require.NoError(t, err, "failed to create pinger")
	pinger.Count = count
	pinger.Timeout = timeout
	pinger.SetPrivileged(true)
	pinger.Run()
	pingStats := pinger.Statistics()
	assert.Equal(t, pinger.Count, pingStats.PacketsRecv, "machine did not respond to all pings")
}
