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
	"os"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/pkg/errors"

	"github.com/firecracker-microvm/firecracker-go-sdk/cni/internal"
)

func main() {
	skel.PluginMain(add, check, del,
		// support CNI versions that support plugin chaining
		version.PluginSupports("0.3.0", "0.3.1", version.Current()),
		buildversion.BuildString("tc-redirect-tap"),
	)
}

func add(args *skel.CmdArgs) error {
	p, err := newPlugin(args)
	if err != nil {
		return err
	}

	currentResult, err := getCurrentResult(args)
	if err != nil {
		return err
	}

	err = p.add(currentResult)
	if err != nil {
		return err
	}

	return types.PrintResult(currentResult, currentResult.CNIVersion)
}

func del(args *skel.CmdArgs) error {
	p, err := newPlugin(args)
	if err != nil {
		return err
	}

	return p.del()
}

func check(args *skel.CmdArgs) error {
	p, err := newPlugin(args)
	if err != nil {
		return err
	}

	return p.check()
}

func getCurrentResult(args *skel.CmdArgs) (*current.Result, error) {
	// parse the previous CNI result (or throw an error if there wasn't one)
	cniConf := types.NetConf{}
	err := json.Unmarshal(args.StdinData, &cniConf)
	if err != nil {
		return nil, errors.Wrap(err, "failure checking for previous result output")
	}

	err = version.ParsePrevResult(&cniConf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse previous CNI result")
	}

	if cniConf.PrevResult == nil {
		return nil, errors.New("no previous result was found, was this plugin chained with a previous one?")
	}

	currentResult, err := current.NewResultFromResult(cniConf.PrevResult)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate current result from previous CNI result")
	}

	return currentResult, nil
}

func newPlugin(args *skel.CmdArgs) (*plugin, error) {
	netNS, err := ns.GetNS(args.Netns)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open netns at path %q", args.Netns)
	}

	if args.IfName == "" {
		return nil, errors.New("no device to redirect with was found, was IfName specified?")
	}

	return &plugin{
		NetlinkOps: internal.DefaultNetlinkOps(),

		// TODO(sipsma) support customizing tap name through args

		// TODO(sipsma) support customizing tap uid/gid through args
		tapUID: os.Geteuid(),
		tapGID: os.Getegid(),

		// given the use case of supporting VMs, we call the "containerID" a "vmID"
		vmID: args.ContainerID,

		redirectInterfaceName: args.IfName,

		netNS: netNS,
	}, nil
}

type plugin struct {
	internal.NetlinkOps

	// vmID is the sandbox ID used to specify the interface that should be created
	// and configured for the VM internally (the CNI spec allows the sandbox ID to
	// be a hypervisor/VM ID in addition to a network namespace path)
	vmID string

	// tapName is the name that the VM's tap device will be created with. If it's
	// unset, it will be with a name decided automatically by the kernel
	tapName string

	// tapUID is the uid of the user-owner of the tap device
	tapUID int

	// tapGID is the gid of the group-owner of the tap device
	tapGID int

	// redirectInterfaceName is the name of the device that the tap device will have a
	// u32 redirect filter pair with. It's provided by the client via the CNI runtime
	// config "IfName" parameter
	redirectInterfaceName string

	// netNS is the network namespace in which the redirectIface exists and thus in which
	// the tap device will be created too
	netNS ns.NetNS
}

func (p plugin) add(currentResult *current.Result) error {
	return p.netNS.Do(func(_ ns.NetNS) error {
		redirectLink, err := p.GetLink(p.redirectInterfaceName)
		if err != nil {
			return errors.Wrapf(err, "failed to find redirect interface %q", p.redirectInterfaceName)
		}

		redirectIPs := internal.InterfaceIPs(currentResult, redirectLink.Attrs().Name, p.netNS.Path())
		if len(redirectIPs) != 1 {
			return errors.Errorf("expected to find 1 IP on redirect interface %q, but instead found %+v",
				redirectLink.Attrs().Name, redirectIPs)
		}
		redirectIP := redirectIPs[0]

		tapLink, err := p.CreateTap(p.tapName, redirectLink.Attrs().MTU, p.tapUID, p.tapGID)
		if err != nil {
			return err
		}

		err = p.AddIngressQdisc(tapLink)
		if err != nil {
			return err
		}

		err = p.AddIngressQdisc(redirectLink)
		if err != nil {
			return err
		}

		err = p.AddRedirectFilter(tapLink, redirectLink)
		if err != nil {
			return err
		}

		err = p.AddRedirectFilter(redirectLink, tapLink)
		if err != nil {
			return err
		}

		// Add the tap device to our results
		currentResult.Interfaces = append(currentResult.Interfaces, &current.Interface{
			Name:    tapLink.Attrs().Name,
			Sandbox: p.netNS.Path(),
			Mac:     tapLink.Attrs().HardwareAddr.String(),
		})

		// Add the pseudo vm interface to our results. It specifies the configuration
		// that should be applied to the VM's internal interface once it is spun up.
		// It is not yet a real device. It is given the same name as the tap device in
		// order to associate it as the internal interface corresponding to the external
		// tap device. However, it's given the vmID as the sandbox ID in order to
		// differentiate from the tap and associate it with the VM.
		//
		// See the `vmconf` package's docstring for the definition of this interface
		currentResult.Interfaces = append(currentResult.Interfaces, &current.Interface{
			Name:    tapLink.Attrs().Name,
			Sandbox: p.vmID,
			Mac:     redirectLink.Attrs().HardwareAddr.String(),
		})
		vmIfaceIndex := len(currentResult.Interfaces) - 1

		// Add the IP configuration that should be applied to the VM internally by
		// associating the IPConfig with the vmIface. We use the redirectIface's IP.
		currentResult.IPs = append(currentResult.IPs, &current.IPConfig{
			Version:   redirectIP.Version,
			Address:   redirectIP.Address,
			Gateway:   redirectIP.Gateway,
			Interface: &vmIfaceIndex,
		})

		return nil
	})
}

func (p plugin) del() error {
	panic("del is currently unimplemented")
}

func (p plugin) check() error {
	panic("check is currently unimplemented")
}
