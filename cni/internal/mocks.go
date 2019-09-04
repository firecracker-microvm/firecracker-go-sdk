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
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

// MockNetlinkOps provides a no-op implementation of the NetlinkOps interface
type MockNetlinkOps struct {
	// CreatedTap is the mock tap device object that will be returned by the mock methods
	CreatedTap netlink.Link

	// RedirectIface is the mock device object that will be returned by the mock methods as the
	// device with which the tap has a filter redirection with.
	RedirectIface netlink.Link

	// AddIngressQdiscErr is an error that will be returned from all AddIngressQdisc calls
	AddIngressQdiscErr error
	// AddRedirectFilterErr is an error that will be returned from all AddRedirectFilterErr calls
	AddRedirectFilterErr error
	// CreateTapErr is an error that will be returned from all CreateTapErr calls
	CreateTapErr error
}

var _ NetlinkOps = &MockNetlinkOps{}

// AddIngressQdisc does nothing and returns an error if configured to do so (otherwise nil)
func (m MockNetlinkOps) AddIngressQdisc(link netlink.Link) error {
	return m.AddIngressQdiscErr
}

// AddRedirectFilter does nothing and returns an error if configured to do so (otherwise nil)
func (m MockNetlinkOps) AddRedirectFilter(sourceLink netlink.Link, targetLink netlink.Link) error {
	return m.AddRedirectFilterErr
}

// GetLink returns CreatedTap if provided the name of CreatedTap, RedirectIface if provided the name
// of RedirectIface or otherwise a netlink.LinkNotFoundError
func (m MockNetlinkOps) GetLink(name string) (netlink.Link, error) {
	switch name {
	case m.RedirectIface.Attrs().Name:
		return m.RedirectIface, nil
	case m.CreatedTap.Attrs().Name:
		return m.CreatedTap, nil
	default:
		return nil, &netlink.LinkNotFoundError{}
	}
}

// CreateTap returns the configured mock tap link and/or a configured error
func (m MockNetlinkOps) CreateTap(name string, mtu int, ownerUID, ownerGID int) (netlink.Link, error) {
	return m.CreatedTap, m.CreateTapErr
}

// MockLink provides a mocked out netlink.Link implementation
type MockLink struct {
	netlink.Link
	netlink.LinkAttrs
}

var _ netlink.Link = &MockLink{}

// Attrs() returns the LinkAttrs configured in the MockLink object
func (l MockLink) Attrs() *netlink.LinkAttrs {
	return &l.LinkAttrs
}

// MockNetNS provides a mocked out ns.NetNS implementation that just executes callbacks
// in the host netns (to avoid permissions issues that require root to resolve).
type MockNetNS struct {
	ns.NetNS
	MockPath string
}

var _ ns.NetNS = &MockNetNS{}

// Do executes the provided callback in the host's netns (it does not actually switch ns)
func (m MockNetNS) Do(f func(ns.NetNS) error) error {
	return f(nil)
}

// Path returns the configured MockPath object in the MockNetNS object
func (m MockNetNS) Path() string {
	return m.MockPath
}
