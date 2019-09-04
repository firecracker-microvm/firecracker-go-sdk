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
	"github.com/containernetworking/cni/pkg/types/current"
)

// InterfaceIPs returns the IPs associated with the interface possessing the provided name and
// sandbox.
func InterfaceIPs(result *current.Result, ifaceName string, sandbox string) []*current.IPConfig {
	var ifaceIPs []*current.IPConfig
	for _, ipconfig := range result.IPs {
		if ipconfig.Interface != nil {
			iface := result.Interfaces[*ipconfig.Interface]
			if iface.Name == ifaceName && iface.Sandbox == sandbox {
				ifaceIPs = append(ifaceIPs, ipconfig)
			}
		}
	}

	return ifaceIPs
}

// FilterBySandbox returns scans the provided list of interfaces and returns two lists: the first
// are a list of interfaces with the provided sandboxID, the second are the other interfaces not
// in that sandboxID.
func FilterBySandbox(
	sandbox string, ifaces ...*current.Interface,
) (in []*current.Interface, out []*current.Interface) {
	for _, iface := range ifaces {
		if iface.Sandbox == sandbox {
			in = append(in, iface)
		} else {
			out = append(out, iface)
		}
	}

	return
}

// IfacesWithName scans the provided list of ifaces and returns the ones with the provided name
func IfacesWithName(name string, ifaces ...*current.Interface) []*current.Interface {
	var foundIfaces []*current.Interface
	for _, iface := range ifaces {
		if iface.Name == name {
			foundIfaces = append(foundIfaces, iface)
		}
	}

	return foundIfaces
}
