package firecracker

import (
	"context"
	"fmt"
)

// Handler name constants
const (
	StartVMMHandlerName                = "StartVMM"
	BootstrapLoggingHandlerName        = "BootstrapLogging"
	CreateMachineHandlerName           = "CreateMachine"
	CreateBootSourceHandlerName        = "CreateBootSource"
	AttachDrivesHandlerName            = "AttachDrives"
	CreateNetworkInterfacesHandlerName = "CreateNetworkInterfaces"
	AddVsocksHandlerName               = "AddVsocks"
)

// StartVMMNamedHandler .
var StartVMMNamedHandler = NamedHandler{
	Name: StartVMMHandlerName,
	Fn: func(ctx context.Context, m *Machine) error {
		m.logger.Debugf(fmt.Sprintf("%s handler executing", StartVMMHandlerName))
		return m.startVMM(ctx)
	},
}

// BootstrapLoggingNamedHandler .
var BootstrapLoggingNamedHandler = NamedHandler{
	Name: BootstrapLoggingHandlerName,
	Fn: func(ctx context.Context, m *Machine) error {
		m.logger.Debugf(fmt.Sprintf("%s handler executing", BootstrapLoggingHandlerName))
		if err := m.setupLogging(ctx); err != nil {
			m.logger.Warnf("setupLogging() returned %s. Continuing anyway.", err)
		} else {
			m.logger.Debugf("back from setupLogging")
		}

		return nil
	},
}

// CreateMachineNamedHandler .
var CreateMachineNamedHandler = NamedHandler{
	Name: CreateMachineHandlerName,
	Fn: func(ctx context.Context, m *Machine) error {
		m.logger.Debugf(fmt.Sprintf("%s handler executing", CreateMachineHandlerName))
		return m.createMachine(ctx)
	},
}

// CreateBootSourceNamedHandler .
var CreateBootSourceNamedHandler = NamedHandler{
	Name: CreateBootSourceHandlerName,
	Fn: func(ctx context.Context, m *Machine) error {
		m.logger.Debugf(fmt.Sprintf("%s handler executing", CreateBootSourceHandlerName))
		return m.createBootSource(ctx, m.cfg.KernelImagePath, m.cfg.KernelArgs)
	},
}

// AttachDrivesNamedHandler .
var AttachDrivesNamedHandler = NamedHandler{
	Name: AttachDrivesHandlerName,
	Fn: func(ctx context.Context, m *Machine) error {
		m.logger.Debugf(fmt.Sprintf("%s handler executing", AttachDrivesHandlerName))
		drives := append([]BlockDevice{m.cfg.RootDrive}, m.cfg.AdditionalDrives...)
		rootIndex := 0

		return m.attachDrives(ctx, rootIndex, drives...)
	},
}

// CreateNetworkInterfacesNamedHandler .
var CreateNetworkInterfacesNamedHandler = NamedHandler{
	Name: CreateNetworkInterfacesHandlerName,
	Fn: func(ctx context.Context, m *Machine) error {
		m.logger.Debugf(fmt.Sprintf("%s handler executing", CreateNetworkInterfacesHandlerName))
		return m.createNetworkInterfaces(ctx, m.cfg.NetworkInterfaces...)
	},
}

// AddVsocksNamedHandler .
var AddVsocksNamedHandler = NamedHandler{
	Name: AddVsocksHandlerName,
	Fn: func(ctx context.Context, m *Machine) error {
		m.logger.Debugf(fmt.Sprintf("%s handler executing", AddVsocksHandlerName))
		return m.addVsocks(ctx, m.cfg.VsockDevices...)
	},
}

var defaultHandlerList = HandlerList{}.Append(
	StartVMMNamedHandler,
	BootstrapLoggingNamedHandler,
	CreateMachineNamedHandler,
	CreateBootSourceNamedHandler,
	AttachDrivesNamedHandler,
	CreateNetworkInterfacesNamedHandler,
	AddVsocksNamedHandler,
)

// HandlerList represents a list of named handler that
// can be used to execute a flow of instructions for a given
// machine.
type HandlerList struct {
	list []NamedHandler
}

// Append will append a new handler to the handler list.
func (l HandlerList) Append(handlers ...NamedHandler) HandlerList {
	l.list = append(l.list, handlers...)

	return l
}

// Remove will return an updated handler with all instances
// of the specific named handler being removed.
func (l HandlerList) Remove(name string) HandlerList {
	newList := HandlerList{}
	for _, h := range l.list {
		if h.Name != name {
			newList.list = append(newList.list, h)
		}
	}

	return newList
}

// Clear clears the whole list of handlers
func (l HandlerList) Clear() HandlerList {
	l.list = l.list[0:0]
	return l
}

// NamedHandler represents a named handler that contains a
// name and a function which is used to execute during
// the initialization process of a machine.
type NamedHandler struct {
	Name string
	Fn   func(context.Context, *Machine) error
}

// Run will execute each instruction in the handler list. If an
// error occurs in any of the handlers, then the list will halt
// execution and return the error.
func (l HandlerList) Run(ctx context.Context, m *Machine) error {
	for _, handler := range l.list {
		if err := handler.Fn(ctx, m); err != nil {
			return err
		}
	}

	return nil
}
