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

// Package main provides a Go program for managing snapshots of Firecracker microVMs.
// The program utilizes the Firecracker Go SDK for creating and loading snapshots,
// and it demonstrates how to establish SSH connections to interact with microVMs.
// Comments are provided to explain each function's purpose and usage.

// In this program, a "snapshot" refers to a point-in-time copy of the state of a Firecracker microVM.
// Snapshots capture the complete memory and state of the microVM, allowing users to save and restore its exact configuration and execution context.
// They enable quick deployment and management of microVM instances with pre-defined configurations and states, which is useful for testing, development, and deployment scenarios.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"

	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

// Constants for CNI configuration
const (
	// Using default cache directory to ensure collision avoidance on IP allocations
	cniCacheDir = "/var/lib/cni" // Default cache directory for IP allocations
	networkName = "fcnet"        // Name of the network
	ifName      = "veth0"        // Interface name

	networkMask = "/24" // Subnet mask
	subnet      = "10.168.0.0" + networkMask

	maxRetries    = 10                     // Maximum number of retries for SSH connection
	backoffTimeMs = 500 * time.Millisecond // Backoff time for retries
)

// writeCNIConfWithHostLocalSubnet writes CNI configuration to a file with a host-local subnet
func writeCNIConfWithHostLocalSubnet(path, networkName, subnet string) error {
	return ioutil.WriteFile(path, []byte(fmt.Sprintf(`{
		"cniVersion": "0.3.1",
		"name": "%s",
		"plugins": [
		  {
			"type": "ptp",
			"ipam": {
			  "type": "host-local",
			  "subnet": "%s"
			}
		  },
		  {
			"type": "tc-redirect-tap"
		  }
		]
	  }`, networkName, subnet)), 0644)
}

// configOpt is a functional option for configuring the Firecracker microVM
type configOpt func(*sdk.Config)

// withNetworkInterface adds a network interface configuration option to the Firecracker microVM config
func withNetworkInterface(networkInterface sdk.NetworkInterface) configOpt {
	return func(c *sdk.Config) {
		c.NetworkInterfaces = append(c.NetworkInterfaces, networkInterface)
	}
}

// createNewConfig creates a new Firecracker microVM configuration
func createNewConfig(socketPath string, opts ...configOpt) sdk.Config {
	dir, _ := os.Getwd()
	fmt.Println(dir)
	kernelImagePath := filepath.Join(dir, "vmlinux")

	var vcpuCount int64 = 2
	var memSizeMib int64 = 256
	smt := false

	driveID := "root"
	isRootDevice := true
	isReadOnly := false
	pathOnHost := "root-drive-with-ssh.img"

	cfg := sdk.Config{
		SocketPath:      socketPath,
		KernelImagePath: kernelImagePath,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  &vcpuCount,
			MemSizeMib: &memSizeMib,
			Smt:        &smt,
		},
		Drives: []models.Drive{
			{
				DriveID:      &driveID,
				IsRootDevice: &isRootDevice,
				IsReadOnly:   &isReadOnly,
				PathOnHost:   &pathOnHost,
			},
		},
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

// connectToVM establishes an SSH connection to the Firecracker microVM
func connectToVM(m *sdk.Machine, sshKeyPath string) (*ssh.Client, error) {
	key, err := ioutil.ReadFile(sshKeyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	if len(m.Cfg.NetworkInterfaces) == 0 {
		return nil, errors.New("No network interfaces")
	}

	ip := m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP // IP of VM

	return ssh.Dial("tcp", fmt.Sprintf("%s:22", ip), config)
}

// createSnapshotSSH creates a snapshot of a Firecracker microVM and returns the IP of the VM
func createSnapshotSSH(ctx context.Context, socketPath, memPath, snapPath string) string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	cniConfDir := filepath.Join(dir, "cni.conf")
	cniBinPath := []string{filepath.Join(dir, "bin")} // CNI binaries

	// Network config
	cniConfPath := fmt.Sprintf("%s/%s.conflist", cniConfDir, networkName)
	err = writeCNIConfWithHostLocalSubnet(cniConfPath, networkName, subnet)
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(cniConfPath)

	networkInterface := sdk.NetworkInterface{
		CNIConfiguration: &sdk.CNIConfiguration{
			NetworkName: networkName,
			IfName:      ifName,
			ConfDir:     cniConfDir,
			BinPath:     cniBinPath,
			VMIfName:    "eth0",
		},
	}

	socketFile := fmt.Sprintf("%s.create", socketPath)

	cfg := createNewConfig(socketFile, withNetworkInterface(networkInterface))

	// Use firecracker binary when making machine
	cmd := sdk.VMCommandBuilder{}.WithSocketPath(socketFile).WithBin(filepath.Join(dir, "firecracker")).Build(ctx)

	m, err := sdk.NewMachine(ctx, cfg, sdk.WithProcessRunner(cmd))
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(socketFile)

	err = m.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.StopVMM(); err != nil {
			log.Fatal(err)
		}
	}()
	defer func() {
		if err := m.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	vmIP := m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP.String()
	fmt.Printf("IP of VM: %v\n", vmIP)

	sshKeyPath := filepath.Join(dir, "root-drive-ssh-key")

	var client *ssh.Client
	for i := 0; i < maxRetries; i++ {
		client, err = connectToVM(m, sshKeyPath)
		if err != nil {
			time.Sleep(backoffTimeMs * time.Millisecond)
		} else {
			break
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		log.Fatal(err)
	}
	defer session.Close()

	fmt.Println(`Sending "sleep 422" command...`)
	err = session.Start(`sleep 422`)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Creating snapshot...")

	err = m.PauseVM(ctx)
	if err != nil {
		log.Fatal(err)
	}

	err = m.CreateSnapshot(ctx, memPath, snapPath)
	if err != nil {
		log.Fatal(err)
	}

	err = m.ResumeVM(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Snapshot created")
	return vmIP
}

// loadSnapshotSSH loads a snapshot of the Firecracker microVM using SSH
func loadSnapshotSSH(ctx context.Context, socketPath, memPath, snapPath, ipToRestore string) {
	var ipFreed bool = false
	var err error

	for i := 0; i < maxRetries; i++ {
		// Wait till the file no longer exists (i.e. os.Stat returns an error)
		if _, err = os.Stat(fmt.Sprintf("%s/networks/%s/%s", cniCacheDir, networkName, ipToRestore)); err == nil {
			time.Sleep(backoffTimeMs * time.Millisecond)
		} else {
			ipFreed = true
			break
		}
	}

	if errors.Is(err, os.ErrNotExist) {
		err = nil
	} else if !ipFreed {
		err = fmt.Errorf("IP %v was not freed", ipToRestore)
	}

	if err != nil {
		log.Fatal(err)
	}

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	cniConfDir := filepath.Join(dir, "cni.conf")
	cniBinPath := []string{filepath.Join(dir, "bin")} // CNI binaries

	// Network config, using the previous machine's IP
	cniConfPath := fmt.Sprintf("%s/%s.conflist", cniConfDir, networkName)
	err = writeCNIConfWithHostLocalSubnet(cniConfPath, networkName, subnet)
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(cniConfPath)

	networkInterface := sdk.NetworkInterface{
		CNIConfiguration: &sdk.CNIConfiguration{
			NetworkName: networkName,
			IfName:      ifName,
			ConfDir:     cniConfDir,
			BinPath:     cniBinPath,
			Args:        [][2]string{{"IP", ipToRestore + networkMask}},
			VMIfName:    "eth0",
		},
	}

	driveID := "root"
	isRootDevice := true
	isReadOnly := false
	rootfsPath := "root-drive-with-ssh.img"

	socketFile := fmt.Sprintf("%s.load", socketPath)
	cfg := sdk.Config{
		SocketPath: socketPath + ".load",
		Drives: []models.Drive{
			{
				DriveID:      &driveID,
				IsRootDevice: &isRootDevice,
				IsReadOnly:   &isReadOnly,
				PathOnHost:   &rootfsPath,
			},
		},
		NetworkInterfaces: []sdk.NetworkInterface{
			networkInterface,
		},
	}

	// Use the firecracker binary
	cmd := sdk.VMCommandBuilder{}.WithSocketPath(socketFile).WithBin(filepath.Join(dir, "firecracker")).Build(ctx)

	m, err := sdk.NewMachine(ctx, cfg, sdk.WithProcessRunner(cmd), sdk.WithSnapshot(memPath, snapPath))
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(socketFile)

	err = m.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.StopVMM(); err != nil {
			log.Fatal(err)
		}
	}()
	defer func() {
		if err := m.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	err = m.ResumeVM(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Snapshot loaded")
	fmt.Printf("IP of VM: %v\n", ipToRestore)

	sshKeyPath := filepath.Join(dir, "root-drive-ssh-key")

	var client *ssh.Client
	for i := 0; i < maxRetries; i++ {
		client, err = connectToVM(m, sshKeyPath)
		if err != nil {
			time.Sleep(backoffTimeMs * time.Millisecond)
		} else {
			break
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		log.Fatal(err)
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	err = session.Run(`ps -aux | grep "sleep 422"`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(b.String())
}

func main() {
	// Check for kvm and root access
	err := unix.Access("/dev/kvm", unix.W_OK) // Check if the program has write access to /dev/kvm
	if err != nil {                           // If there's an error (e.g., access denied), log and exit
		log.Fatal(err)
	}

	// Check if the program is running with root privileges
	if x, y := 0, os.Getuid(); x != y {
		log.Fatal("Root acccess denied")
	}

	// Get the current working directory
	dir, err := os.Getwd()
	if err != nil { // If there's an error getting the working directory, log and exit
		log.Fatal(err)
	}

	// Create a directory for CNI configuration files
	cniConfDir := filepath.Join(dir, "cni.conf")
	err = os.Mkdir(cniConfDir, 0777) // Create the directory with full permissions
	if err != nil {                  // If there's an error creating the directory, log and exit
		log.Fatal(err)
	}
	defer os.Remove(cniConfDir) // Remove the directory when main function exits

	// Setup temporary directory and paths for socket, snapshot, and memory files
	tempdir, err := ioutil.TempDir("", "FCGoSDKSnapshotExample") // Create a temporary directory
	if err != nil {                                              // If there's an error creating the temporary directory, log and exit
		log.Fatal(err)
	}
	defer os.Remove(tempdir)                            // Remove the temporary directory when main function exits
	socketPath := filepath.Join(tempdir, "snapshotssh") // Create a socket path within the temporary directory

	// Create a directory for snapshot and memory files
	snapshotsshPath := filepath.Join(dir, "snapshotssh")
	err = os.Mkdir(snapshotsshPath, 0777)           // Create the directory with full permissions
	if err != nil && !errors.Is(err, os.ErrExist) { // If there's an error creating the directory and it's not already exist, log and exit
		log.Fatal(err)
	}
	defer os.RemoveAll(snapshotsshPath) // Remove the directory and its contents when main function exits

	// Set paths for snapshot and memory files
	snapPath := filepath.Join(snapshotsshPath, "SnapFile")
	memPath := filepath.Join(snapshotsshPath, "MemFile")

	// Create a background context
	ctx := context.Background()

	// Create a snapshot of the Firecracker microVM and get the IP address to restore
	ipToRestore := createSnapshotSSH(ctx, socketPath, memPath, snapPath)
	fmt.Println()

	// Load the snapshot of the Firecracker microVM
	loadSnapshotSSH(ctx, socketPath, memPath, snapPath, ipToRestore)
}
