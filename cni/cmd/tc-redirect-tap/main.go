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
	pluginargs "github.com/firecracker-microvm/firecracker-go-sdk/cni/cmd/tc-redirect-tap/args"
	"os"
	"strconv"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/hashicorp/go-multierror"
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

	if p.netNS == nil {
		return errors.Errorf("netns path %q does not exist", args.Netns)
	}

	if p.currentResult == nil {
		return &NoPreviousResultError{}
	}

	err = p.add()
	if err != nil {
		return err
	}

	return types.PrintResult(p.currentResult, p.currentResult.CNIVersion)
}

func del(args *skel.CmdArgs) error {
	p, err := newPlugin(args)
	if err != nil {
		return err
	}

	if p.netNS == nil {
		// the network namespace is already gone and everything we create
		// is inside the netns, so nothing to do here.
		return nil
	}

	return p.del()
}

func check(args *skel.CmdArgs) error {
	p, err := newPlugin(args)
	if err != nil {
		return err
	}

	if p.netNS == nil {
		return errors.Errorf("netns path %q does not exist", args.Netns)
	}

	if p.currentResult == nil {
		return &NoPreviousResultError{}
	}

	return p.check()
}

func newPlugin(args *skel.CmdArgs) (*plugin, error) {
	if args.IfName == "" {
		return nil, errors.New("no device to redirect with was found, was IfName specified?")
	}

	netNS, err := ns.GetNS(args.Netns)
	if err != nil {
		// It's valid for the netns to no longer exist during DEL commands (in which case DEL is
		// a noop). Thus, we leave validating that netNS is not nil to command implementations.
		switch err.(type) {
		case ns.NSPathNotExistErr:
			netNS = nil
		default:
			return nil, errors.Wrapf(err, "failed to open netns at path %q", args.Netns)
		}
	}

	currentResult, err := getCurrentResult(args)
	if err != nil {
		switch err.(type) {
		case *NoPreviousResultError:
			currentResult = nil
		default:
			return nil, errors.Wrapf(err, "failure parsing previous CNI result")
		}
	}

	plugin := &plugin{
		NetlinkOps: internal.DefaultNetlinkOps(),
		tapUID:     os.Geteuid(),
		tapGID:     os.Getegid(),

		// given the use case of supporting VMs, we call the "containerID" a "vmID"
		vmID: args.ContainerID,

		redirectInterfaceName: args.IfName,

		netNS: netNS,

		currentResult: currentResult,
	}
	parsedArgs, err := extractArgs(args.Args)
	if err != nil {
		return nil, err
	}

	if tapName, wasDefined := parsedArgs[pluginargs.TCRedirectTapName]; wasDefined {
		plugin.tapName = tapName
	}

	if tapUIDVal, wasDefined := parsedArgs[pluginargs.TCRedirectTapUID]; wasDefined {
		tapUID, err := strconv.Atoi(tapUIDVal)
		if err != nil {
			return nil, errors.Wrapf(err, "tapUID should be numeric convertible, got %q", tapUIDVal)
		}
		plugin.tapUID = tapUID
	}

	if tapGIDVal, wasDefined := parsedArgs[pluginargs.TCRedirectTapGUID]; wasDefined {
		tapGID, err := strconv.Atoi(tapGIDVal)
		if err != nil {
			return nil, errors.Wrapf(err, "tapGID should be numeric convertible, got %q", tapGIDVal)
		}
		plugin.tapGID = tapGID
	}

	return plugin, nil
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
		return nil, &NoPreviousResultError{}
	}

	currentResult, err := current.NewResultFromResult(cniConf.PrevResult)
	if err != nil {
		return nil, errors.Wrap(err,
			"failed to generate current result from previous CNI result")
	}

	return currentResult, nil
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

	// currentResult is the CNI result object, initialized to the previous CNI
	// result if there was any or to nil if there was no previous result provided
	currentResult *current.Result
}

func (p plugin) add() error {
	return p.netNS.Do(func(_ ns.NetNS) error {
		redirectLink, err := p.GetLink(p.redirectInterfaceName)
		if err != nil {
			return errors.Wrapf(err,
				"failed to find redirect interface %q", p.redirectInterfaceName)
		}

		redirectIPs := internal.InterfaceIPs(
			p.currentResult, redirectLink.Attrs().Name, p.netNS.Path())
		if len(redirectIPs) != 1 {
			return errors.Errorf(
				"expected to find 1 IP on redirect interface %q, but instead found %+v",
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
		p.currentResult.Interfaces = append(p.currentResult.Interfaces, &current.Interface{
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
		p.currentResult.Interfaces = append(p.currentResult.Interfaces, &current.Interface{
			Name:    tapLink.Attrs().Name,
			Sandbox: p.vmID,
			Mac:     redirectLink.Attrs().HardwareAddr.String(),
		})
		vmIfaceIndex := len(p.currentResult.Interfaces) - 1

		// Add the IP configuration that should be applied to the VM internally by
		// associating the IPConfig with the vmIface. We use the redirectIface's IP.
		p.currentResult.IPs = append(p.currentResult.IPs, &current.IPConfig{
			Version:   redirectIP.Version,
			Address:   redirectIP.Address,
			Gateway:   redirectIP.Gateway,
			Interface: &vmIfaceIndex,
		})

		return nil
	})
}

func (p plugin) del() error {
	return p.netNS.Do(func(_ ns.NetNS) error {
		var multiErr *multierror.Error

		// try to remove the qdisc we added from the redirect interface
		redirectLink, err := p.GetLink(p.redirectInterfaceName)
		switch err.(type) {

		case nil:
			// the link exists, so try removing the qdisc
			err := p.RemoveIngressQdisc(redirectLink)
			switch err.(type) {
			case nil, *internal.QdiscNotFoundError:
				// we removed successfully or there already wasn't a qdisc, nothing to do
			default:
				multiErr = multierror.Append(multiErr, errors.Wrapf(err,
					"failed to remove ingress qdisc from %q", redirectLink.Attrs().Name))
			}

		case *internal.LinkNotFoundError:
			// if the link doesn't exist, there's nothing to do

		default:
			multiErr = multierror.Append(multiErr,
				errors.Wrapf(err, "failure finding device %q", p.redirectInterfaceName))
		}

		// if there was no previous result, we can't find the vm-tap pair, so we are done here
		if p.currentResult == nil {
			return multiErr.ErrorOrNil()
		}

		// try to remove the tap device we added
		_, tapIface, err := internal.VMTapPair(p.currentResult, p.vmID)
		switch err.(type) {

		case nil:
			err = p.RemoveLink(tapIface.Name)
			switch err.(type) {
			case nil, *internal.LinkNotFoundError:
				// we removed successfully or someone else beat us to removing it first
			default:
				multiErr = multierror.Append(multiErr, errors.Wrapf(err,
					"failure removing device %q", tapIface.Name))
			}

		case *internal.LinkNotFoundError:
			// if the link doesn't exist, there's nothing to do

		default:
			multiErr = multierror.Append(multiErr, err)
		}

		return multiErr.ErrorOrNil()
	})
}

func (p plugin) check() error {
	return p.netNS.Do(func(_ ns.NetNS) error {
		_, tapIface, err := internal.VMTapPair(p.currentResult, p.vmID)
		if err != nil {
			return err
		}

		tapLink, err := p.GetLink(tapIface.Name)
		if err != nil {
			return err
		}

		redirectLink, err := p.GetLink(p.redirectInterfaceName)
		if err != nil {
			return err
		}

		_, err = p.GetIngressQdisc(tapLink)
		if err != nil {
			return err
		}

		_, err = p.GetIngressQdisc(redirectLink)
		if err != nil {
			return err
		}

		_, err = p.GetRedirectFilter(tapLink, redirectLink)
		if err != nil {
			return err
		}

		_, err = p.GetRedirectFilter(redirectLink, tapLink)
		if err != nil {
			return err
		}

		return nil
	})
}

type NoPreviousResultError struct{}

func (e NoPreviousResultError) Error() string {
	return "no previous result was found, was this plugin chained with a previous one?"
}

// extractArgs returns cli args in form of map of strings
// args string - cli args string("key1=val1;key2=val2)
func extractArgs(args string) (map[string]string, error) {
	result := make(map[string]string)
	if args != "" {
		argumentsPairs := strings.Split(args, ";")
		for _, pairStr := range argumentsPairs {
			pair := strings.SplitN(pairStr, "=", 2)
			if len(pair) < 2 {
				return result, errors.Errorf("Invalid cni arguments format, %q", pairStr)
			}
			result[pair[0]] = pair[1]
		}
	}

	return result, nil
}
