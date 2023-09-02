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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"

	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

const (
	// Using default cache directory to ensure collision avoidance on IP allocations
	cniCacheDir = "/var/lib/cni"
	networkName = "fcnet"
	ifName      = "veth0"

	networkMask string = "/24"
	subnet      string = "10.168.0.0" + networkMask

	maxRetries    int           = 10
	backoffTimeMs time.Duration = 500
)

func writeCNIConfWithHostLocalSubnet(path, networkName, subnet string) error {
	return os.WriteFile(path, []byte(fmt.Sprintf(`{
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

type configOpt func(*sdk.Config)

func withNetworkInterface(networkInterface sdk.NetworkInterface) configOpt {
	return func(c *sdk.Config) {
		c.NetworkInterfaces = append(c.NetworkInterfaces, networkInterface)
	}
}

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

func connectToVM(m *sdk.Machine, sshKeyPath string) (*ssh.Client, error) {
	key, err := os.ReadFile(sshKeyPath)
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
	err := unix.Access("/dev/kvm", unix.W_OK)
	if err != nil {
		log.Fatal(err)
	}

	if x, y := 0, os.Getuid(); x != y {
		log.Fatal("Root acccess denied")
	}

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	cniConfDir := filepath.Join(dir, "cni.conf")
	err = os.Mkdir(cniConfDir, 0777)
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(cniConfDir)

	// Setup socket and snapshot + memory paths
	tempdir, err := os.MkdirTemp("", "FCGoSDKSnapshotExample")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tempdir)
	socketPath := filepath.Join(tempdir, "snapshotssh")

	snapshotsshPath := filepath.Join(dir, "snapshotssh")
	err = os.Mkdir(snapshotsshPath, 0777)
	if err != nil && !errors.Is(err, os.ErrExist) {
		log.Fatal(err)
	}
	defer os.RemoveAll(snapshotsshPath)

	snapPath := filepath.Join(snapshotsshPath, "SnapFile")
	memPath := filepath.Join(snapshotsshPath, "MemFile")

	ctx := context.Background()

	ipToRestore := createSnapshotSSH(ctx, socketPath, memPath, snapPath)
	fmt.Println()
	loadSnapshotSSH(ctx, socketPath, memPath, snapPath, ipToRestore)
}
