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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

const (
	terminalProgram = "xterm"
	// consoleXterm indicates that the machine's console should be presented in an xterm
	consoleXterm = "xterm"
	// consoleStdio indicates that the machine's console should re-use the parent's stdio streams
	consoleStdio = "stdio"
	// consoleFile inddicates that the machine's console should be presented in files rather than stdout/stderr
	consoleFile = "file"
	// consoleNone indicates that the machine's console IO should be discarded
	consoleNone = "none"

	// executableMask is the mask needed to check whether or not a file's
	// permissions are executable.
	executableMask = 0111
)

func parseBlockDevices(entries []string) ([]firecracker.BlockDevice, error) {
	var devices []firecracker.BlockDevice
	for _, entry := range entries {
		var path string
		if strings.HasSuffix(entry, ":rw") {
			path = strings.TrimSuffix(entry, ":rw")
		} else if strings.HasSuffix(entry, ":ro") {
			path = strings.TrimSuffix(entry, ":ro")
		} else {
			msg := fmt.Sprintf("Invalid drive specification. Must have :rw or :ro suffix")
			return []firecracker.BlockDevice{}, errors.New(msg)
		}
		if path == "" {
			return nil, errors.New("Invalid drive specification")
		}
		_, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		e := firecracker.BlockDevice{
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
func parseVsocks(devices []string) ([]firecracker.VsockDevice, error) {
	var result []firecracker.VsockDevice
	for _, entry := range devices {
		fields := strings.Split(entry, ":")
		if len(fields) != 2 {
			return []firecracker.VsockDevice{}, errors.New("Could not parse")
		}
		CID, err := strconv.ParseUint(fields[1], 10, 32)
		if err != nil {
			return []firecracker.VsockDevice{}, errors.New("Vsock CID could not be parsed as a number")
		}
		dev := firecracker.VsockDevice{
			Path: fields[0],
			CID:  uint32(CID),
		}
		result = append(result, dev)
	}
	return result, nil
}

type options struct {
	FcBinary           string   `long:"firecracker-binary" description:"Path to firecracker binary"`
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
	FcCaptureFifoLogs  bool     `long:"capture-fifo-logs" description:"pipes fifo and metric fifo's contents to files"`
	FcDisableHt        bool     `long:"disable-hyperthreading" short:"t" description:"Disable CPU Hyperthreading"`
	FcCPUCount         int64    `long:"ncpus" short:"c" description:"Number of CPUs" default:"1"`
	FcCPUTemplate      string   `long:"cpu-template" description:"Firecracker CPU Template (C3 or T2)"`
	FcMemSz            int64    `long:"memory" short:"m" description:"VM memory, in MiB" default:"512"`
	FcMetadata         string   `long:"metadata" description:"Firecracker Meatadata for MMDS (json)"`
	Debug              bool     `long:"debug" short:"d" description:"Enable debug output"`
	Help               bool     `long:"help" short:"h" description:"Show usage"`
}

func main() {
	var err error
	opts := options{}

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

	var metadata interface{}
	if opts.FcMetadata != "" {
		if err := json.Unmarshal([]byte(opts.FcMetadata), &metadata); err != nil {
			log.Fatalf("Unable to parse metadata as json: %s", err)
		}
	}

	var NICs []firecracker.NetworkInterface

	if len(opts.FcNicConfig) > 0 {
		tapDev, tapMacAddr, err := parseNicConfig(opts.FcNicConfig)
		if err != nil {
			log.Fatalf("Unable to parse NIC config: %s", err)
		} else {
			log.Printf("Adding tap device %s", tapDev)
			allowMDDS := metadata != nil
			NICs = []firecracker.NetworkInterface{
				firecracker.NetworkInterface{
					MacAddress:  tapMacAddr,
					HostDevName: tapDev,
					AllowMDDS:   allowMDDS,
				},
			}
		}
	}

	rootDrive := firecracker.BlockDevice{HostPath: opts.FcRootDrivePath, Mode: "rw"}

	blockDevices, err := parseBlockDevices(opts.FcAdditionalDrives)
	if err != nil {
		log.Fatalf("Invalid block device specification: %s", err)
	}

	vsocks, err := parseVsocks(opts.FcVsockDevices)
	if err != nil {
		log.Fatalf("Invalid vsock specification: %s", err)
	}

	fcCfg := firecracker.Config{
		SocketPath:            "./firecracker.sock",
		LogFifo:               opts.FcLogFifo,
		LogLevel:              opts.FcLogLevel,
		MetricsFifo:           opts.FcMetricsFifo,
		CaptureFifoLogsToFile: opts.FcCaptureFifoLogs,
		KernelImagePath:       opts.FcKernelImage,
		KernelArgs:            opts.FcKernelCmdLine,
		RootDrive:             rootDrive,
		RootPartitionUUID:     opts.FcRootPartUUID,
		AdditionalDrives:      blockDevices,
		NetworkInterfaces:     NICs,
		VsockDevices:          vsocks,
		CPUCount:              opts.FcCPUCount,
		CPUTemplate:           firecracker.CPUTemplate(opts.FcCPUTemplate),
		HtEnabled:             !opts.FcDisableHt,
		MemInMiB:              opts.FcMemSz,
		Debug:                 opts.Debug,
	}

	if len(os.Args) == 1 {
		p.WriteHelp(os.Stderr)
		os.Exit(0)
	}

	ctx := context.Background()
	vmmCtx, vmmCancel := context.WithCancel(ctx)
	defer vmmCancel()

	machineOpts := []firecracker.Opt{
		firecracker.WithLogger(log.NewEntry(logger)),
	}

	if len(opts.FcBinary) != 0 {
		finfo, err := os.Stat(opts.FcBinary)
		if os.IsNotExist(err) {
			log.Fatalf("Binary, %q, does not exist: %v", opts.FcBinary, err)
		}

		if err != nil {
			log.Fatalf("Failed to stat binary, %q: %v", opts.FcBinary, err)
		}

		if finfo.IsDir() {
			log.Fatalf("Binary, %q, is a directory", opts.FcBinary)
		} else if finfo.Mode()&executableMask == 0 {
			log.Fatalf("Binary, %q, is not executable. Check permissions of binary", opts.FcBinary)
		}

		cmd := firecracker.VMCommandBuilder{}.
			WithBin(opts.FcBinary).
			WithSocketPath(fcCfg.SocketPath).
			WithStdin(os.Stdin).
			WithStdout(os.Stdout).
			WithStderr(os.Stderr).
			Build(ctx)

		machineOpts = append(machineOpts, firecracker.WithProcessRunner(cmd))
	}

	m, err := firecracker.NewMachine(fcCfg, machineOpts...)
	if err != nil {
		log.Fatalf("Failed creating machine: %s", err)
	}

	if err := m.Init(vmmCtx); err != nil {
		log.Fatalf("Firecracker Init returned error %s", err)
	}

	if metadata != nil {
		err := m.SetMetadata(vmmCtx, metadata)
		if err != nil {
			log.Fatalf("Firecracker SetMetadata returned error %s", err)
		}
	}

	if err := m.StartInstance(vmmCtx); err != nil {
		log.Fatalf("Failed to start instance: %v", err)
	}

	// wait for the VMM to exit
	if err := <-m.ErrCh; err != nil {
		log.Fatalf("startVMM returned error %s", err)
	}
	log.Printf("startVMM was happy")
}
