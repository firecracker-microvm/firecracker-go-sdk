// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/awslabs/go-firecracker"
	flags "github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

func validateDriveEntry(entry string) error {

	return nil
}

func checkConfig(cfg machine.Config) error {
	var err error

	// Check for the existence of some required files:
	_, err = os.Stat(cfg.BinPath)
	if err != nil {
		return err
	}
	_, err = os.Stat(cfg.KernelImagePath)
	if err != nil {
		return err
	}
	_, err = os.Stat(cfg.RootDrive.HostPath)
	if err != nil {
		return err
	}

	// Check the non-existence of some files:
	_, err = os.Stat(cfg.SocketPath)
	if err == nil {
		msg := fmt.Sprintf("Socket %s already exists.", cfg.SocketPath)
		return errors.New(msg)
	}
	return nil
}

func parseBlockDevices(entries []string) ([]machine.BlockDevice, error) {
	var devices []machine.BlockDevice
	for _, entry := range entries {
		var path string
		if strings.HasSuffix(entry, ":rw") {
			path = strings.TrimSuffix(entry, ":rw")
		} else if strings.HasSuffix(entry, ":ro") {
			path = strings.TrimSuffix(entry, ":ro")
		} else {
			msg := fmt.Sprintf("Invalid drive specification. Must have :rw or :ro suffix")
			return []machine.BlockDevice{}, errors.New(msg)
		}
		if path == "" {
			return nil, errors.New("Invalid drive specification")
		}
		_, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		e := machine.BlockDevice{
			HostPath: path,
			Mode:     "rw",
		}
		devices = append(devices, e)
	}
	return devices, nil
}

// Given a string of the form DEVICE/MACADDR, return the device name and the mac address, or an error
func parseNicConfig(cfg string) (string, string, error) {
	// We've really only got one error:
	err := errors.New("NIC config wasn't of the form DEVICE/MACADDR")
	fields := strings.Split(cfg, "/")
	// This isn't the most sophisticated input validation, but this program is just a demo...
	if len(fields) != 2 {
		return "", "", err
	}
	return fields[0], fields[1], nil
}

// Given a list of string representations of vsock devices,
// return a corresponding slice of machine.VsockDevice objects
func parseVsocks(devices []string) ([]machine.VsockDevice, error) {
	var result []machine.VsockDevice
	for _, entry := range devices {
		fields := strings.Split(entry, ":")
		if len(fields) != 2 {
			return []machine.VsockDevice{}, errors.New("Could not parse")
		}
		CID, err := strconv.ParseUint(fields[1], 10, 32)
		if err != nil {
			return []machine.VsockDevice{}, errors.New("Vsock CID could not be parsed as a number")
		}
		dev := machine.VsockDevice{
			Path: fields[0],
			CID:  uint32(CID),
		}
		result = append(result, dev)
	}
	return result, nil
}

func main() {
	var err error
	var opts struct {
		FcBinary           string   `long:"firecracker-binary" description:"Path to firecracker binary"`
		FcConsole          string   `long:"firecracker-console" description:"Console type (stdio|xterm|none)" default:"stdio"`
		FcKernelImage      string   `long:"kernel" description:"Path to the kernel image" default:"./vmlinux"`
		FcKernelCmdLine    string   `long:"kernel-opts" description:"Kernel commandline" default:"ro console=ttyS0 noapic reboot=k panic=1 pci=off nomodules"`
		FcRootDrivePath    string   `long:"root-drive" description:"Path to root disk image"`
		FcRootPartUUID     string   `long:"root-partition" description:"Root partition UUID"`
		FcAdditionalDrives []string `long:"add-drive" description:"Path to additional drive, suffixed with :ro or :rw, can be specified multiple times"`
		FcNicConfig        string   `long:"tap-device" description:"NIC info, specified as DEVICE/MAC"`
		FcVsockDevices     []string `long:"vsock-device" description:"Vsock interface, specified as PATH:CID. Multiple OK"`
		FcLogFifo          string   `long:"vmm-log-fifo" description:"FIFO for firecracker logs"`
		FcLogLevel         string   `long:"log-level" description:"vmm log level" default:"Debug"`
		FcMetricsFifo      string   `long:"metrics-fifo" description:"FIFO for firecracker metrics"`
		FcDisableHt        bool     `long:"disable-hyperthreading" short:"t" description:"Disable CPU Hyperthreading"`
		FcCPUCount         int64    `long:"ncpus" short:"c" description:"Number of CPUs" default:"1"`
		FcCPUTemplate      string   `long:"cpu-template" description:"Firecracker CPU Template (C3 or T2)"`
		FcMemSz            int64    `long:"memory" short:"m" description:"VM memory, in MiB" default:"512"`
		Debug              bool     `long:"debug" short:"d" description:"Enable debug output"`
		Help               bool     `long:"help" short:"h" description:"Show usage"`
	}

	p := flags.NewParser(&opts, 0)
	_, err = p.Parse()
	if err != nil {
		log.Errorf("Error: %s", err)
		p.WriteHelp(os.Stderr)
		os.Exit(1)
	}

	if opts.Help {
		p.WriteHelp(os.Stderr)
		os.Exit(0)
	}

	logger := log.New()

	if opts.Debug {
		log.SetLevel(log.DebugLevel)
		logger.SetLevel(log.DebugLevel)
	}

	var NICs []machine.NetworkInterface

	if len(opts.FcNicConfig) > 0 {
		tapDev, tapMacAddr, err := parseNicConfig(opts.FcNicConfig)
		if err != nil {
			log.Fatalf("Unable to parse NIC config: %s", err)
		} else {
			log.Printf("Adding tap device %s", tapDev)
			NICs = []machine.NetworkInterface{
				machine.NetworkInterface{
					MacAddress:  tapMacAddr,
					HostDevName: tapDev,
				},
			}
		}
	}

	rootDrive := machine.BlockDevice{HostPath: opts.FcRootDrivePath, Mode: "rw"}

	blockDevices, err := parseBlockDevices(opts.FcAdditionalDrives)
	if err != nil {
		log.Fatalf("Invalid block device specification: %s", err)
	}

	vsocks, err := parseVsocks(opts.FcVsockDevices)
	if err != nil {
		log.Fatalf("Invalid vsock specification: %s", err)
	}

	fcCfg := machine.Config{
		BinPath:           opts.FcBinary,
		SocketPath:        "./firecracker.sock",
		LogFifo:           opts.FcLogFifo,
		LogLevel:          opts.FcLogLevel,
		MetricsFifo:       opts.FcMetricsFifo,
		KernelImagePath:   opts.FcKernelImage,
		KernelArgs:        opts.FcKernelCmdLine,
		RootDrive:         rootDrive,
		RootPartitionUUID: opts.FcRootPartUUID,
		AdditionalDrives:  blockDevices,
		NetworkInterfaces: NICs,
		VsockDevices:      vsocks,
		Console:           opts.FcConsole,
		CPUCount:          opts.FcCPUCount,
		CPUTemplate:       machine.CPUTemplate(opts.FcCPUTemplate),
		HtEnabled:         !opts.FcDisableHt,
		MemInMiB:          opts.FcMemSz,
	}

	err = checkConfig(fcCfg)
	if err != nil {
		log.Fatalf("Configuration error: %s", err)
	}

	fireracker := machine.NewFirecrackerClient(fcCfg.SocketPath)
	m := machine.NewMachine(fcCfg, fireracker, logger)

	ctx := context.Background()
	vmmCtx, vmmCancel := context.WithCancel(ctx)
	defer vmmCancel()
	errchan, err := m.Init(ctx)
	if err != nil {
		log.Fatalf("Firecracker Init returned error %s", err)
	} else {
		m.StartInstance(vmmCtx)
	}

	// wait for the VMM to exit
	err = <-errchan
	if err != nil {
		log.Fatalf("startVMM returned error %s", err)
	} else {
		log.Printf("startVMM was happy")
	}
}
