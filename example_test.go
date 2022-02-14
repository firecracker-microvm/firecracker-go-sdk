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
package firecracker_test

import (
	"context"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

func ExampleWithProcessRunner_logging() {
	const socketPath = "/tmp/firecracker.sock"

	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: "/path/to/kernel",
		Drives:          firecracker.NewDrivesBuilder("/path/to/rootfs").Build(),
		MachineCfg: models.MachineConfiguration{
			VcpuCount: firecracker.Int64(1),
		},
	}

	// stdout will be directed to this file
	stdoutPath := "/tmp/stdout.log"
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Errorf("failed to create stdout file: %v", err))
	}

	// stderr will be directed to this file
	stderrPath := "/tmp/stderr.log"
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Errorf("failed to create stderr file: %v", err))
	}

	ctx := context.Background()
	// build our custom command that contains our two files to
	// write to during process execution
	cmd := firecracker.VMCommandBuilder{}.
		WithBin("firecracker").
		WithSocketPath(socketPath).
		WithStdout(stdout).
		WithStderr(stderr).
		Build(ctx)

	m, err := firecracker.NewMachine(ctx, cfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		panic(fmt.Errorf("failed to create new machine: %v", err))
	}

	defer os.Remove(cfg.SocketPath)

	if err := m.Start(ctx); err != nil {
		panic(fmt.Errorf("failed to initialize machine: %v", err))
	}

	// wait for VMM to execute
	if err := m.Wait(ctx); err != nil {
		panic(err)
	}
}

func ExampleDrivesBuilder() {
	drivesParams := []struct {
		Path     string
		ReadOnly bool
	}{
		{
			Path:     "/first/path/drive.img",
			ReadOnly: true,
		},
		{
			Path:     "/second/path/drive.img",
			ReadOnly: false,
		},
	}

	// construct a new builder with the given rootfs path
	b := firecracker.NewDrivesBuilder("/path/to/rootfs")
	for _, param := range drivesParams {
		// add our additional drives
		b = b.AddDrive(param.Path, param.ReadOnly)
	}

	const socketPath = "/tmp/firecracker.sock"
	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: "/path/to/kernel",
		// build our drives into the machine's configuration
		Drives: b.Build(),
		MachineCfg: models.MachineConfiguration{
			VcpuCount: firecracker.Int64(1),
		},
	}

	ctx := context.Background()
	m, err := firecracker.NewMachine(ctx, cfg)
	if err != nil {
		panic(fmt.Errorf("failed to create new machine: %v", err))
	}

	if err := m.Start(ctx); err != nil {
		panic(fmt.Errorf("failed to initialize machine: %v", err))
	}

	// wait for VMM to execute
	if err := m.Wait(ctx); err != nil {
		panic(err)
	}
}

func ExampleDrivesBuilder_driveOpt() {
	drives := firecracker.NewDrivesBuilder("/path/to/rootfs").
		AddDrive("/path/to/drive1.img", true).
		AddDrive("/path/to/drive2.img", false, func(drive *models.Drive) {
			// set our custom bandwidth rate limiter
			drive.RateLimiter = &models.RateLimiter{
				Bandwidth: &models.TokenBucket{
					OneTimeBurst: firecracker.Int64(1024 * 1024),
					RefillTime:   firecracker.Int64(500),
					Size:         firecracker.Int64(1024 * 1024),
				},
			}
		}).
		Build()

	const socketPath = "/tmp/firecracker.sock"
	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: "/path/to/kernel",
		// build our drives into the machine's configuration
		Drives: drives,
		MachineCfg: models.MachineConfiguration{
			VcpuCount: firecracker.Int64(1),
		},
	}

	ctx := context.Background()
	m, err := firecracker.NewMachine(ctx, cfg)
	if err != nil {
		panic(fmt.Errorf("failed to create new machine: %v", err))
	}

	if err := m.Start(ctx); err != nil {
		panic(fmt.Errorf("failed to initialize machine: %v", err))
	}

	// wait for VMM to execute
	if err := m.Wait(ctx); err != nil {
		panic(err)
	}
}

func ExampleNetworkInterface_rateLimiting() {
	// construct the limitations of the bandwidth for firecracker
	bandwidthBuilder := firecracker.TokenBucketBuilder{}.
		WithInitialSize(1024 * 1024).        // Initial token amount
		WithBucketSize(1024 * 1024).         // Max number of tokens
		WithRefillDuration(30 * time.Second) // Refill rate

	// construct the limitations of the number of operations per duration for firecracker
	opsBuilder := firecracker.TokenBucketBuilder{}.
		WithInitialSize(5).
		WithBucketSize(5).
		WithRefillDuration(5 * time.Second)

	// create the inbound rate limiter
	inbound := firecracker.NewRateLimiter(bandwidthBuilder.Build(), opsBuilder.Build())

	bandwidthBuilder = bandwidthBuilder.WithBucketSize(1024 * 1024 * 10)
	opsBuilder = opsBuilder.
		WithBucketSize(100).
		WithInitialSize(100)
	// create the outbound rate limiter
	outbound := firecracker.NewRateLimiter(bandwidthBuilder.Build(), opsBuilder.Build())

	networkIfaces := []firecracker.NetworkInterface{{
		StaticConfiguration: &firecracker.StaticNetworkConfiguration{
			MacAddress:  "01-23-45-67-89-AB-CD-EF",
			HostDevName: "tap-name",
		},
		InRateLimiter:  inbound,
		OutRateLimiter: outbound,
	}}

	cfg := firecracker.Config{
		SocketPath:      "/path/to/socket",
		KernelImagePath: "/path/to/kernel",
		Drives:          firecracker.NewDrivesBuilder("/path/to/rootfs").Build(),
		MachineCfg: models.MachineConfiguration{
			VcpuCount: firecracker.Int64(1),
		},
		NetworkInterfaces: networkIfaces,
	}

	ctx := context.Background()
	m, err := firecracker.NewMachine(ctx, cfg)
	if err != nil {
		panic(fmt.Errorf("failed to create new machine: %v", err))
	}

	defer os.Remove(cfg.SocketPath)

	if err := m.Start(ctx); err != nil {
		panic(fmt.Errorf("failed to initialize machine: %v", err))
	}

	// wait for VMM to execute
	if err := m.Wait(ctx); err != nil {
		panic(err)
	}
}

func ExampleJailerConfig_enablingJailer() {
	ctx := context.Background()
	vmmCtx, vmmCancel := context.WithCancel(ctx)
	defer vmmCancel()

	const id = "my-jailer-test"
	const path = "/path/to/jailer-workspace"
	const kernelImagePath = "/path/to/kernel-image"

	uid := 123
	gid := 100

	fcCfg := firecracker.Config{
		SocketPath:      "api.socket",
		KernelImagePath: kernelImagePath,
		KernelArgs:      "console=ttyS0 reboot=k panic=1 pci=off",
		Drives:          firecracker.NewDrivesBuilder("/path/to/rootfs").Build(),
		LogLevel:        "Debug",
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(1),
			Smt:        firecracker.Bool(false),
			MemSizeMib: firecracker.Int64(256),
		},
		JailerCfg: &firecracker.JailerConfig{
			UID:            &uid,
			GID:            &gid,
			ID:             id,
			NumaNode:       firecracker.Int(0),
			ChrootBaseDir:  path,
			ChrootStrategy: firecracker.NewNaiveChrootStrategy(kernelImagePath),
			ExecFile:       "/path/to/firecracker-binary",
		},
	}

	// Check if kernel image is readable
	f, err := os.Open(fcCfg.KernelImagePath)
	if err != nil {
		panic(fmt.Errorf("Failed to open kernel image: %v", err))
	}
	f.Close()

	// Check each drive is readable and writable
	for _, drive := range fcCfg.Drives {
		drivePath := firecracker.StringValue(drive.PathOnHost)
		f, err := os.OpenFile(drivePath, os.O_RDWR, 0666)
		if err != nil {
			panic(fmt.Errorf("Failed to open drive with read/write permissions: %v", err))
		}
		f.Close()
	}

	logger := log.New()
	m, err := firecracker.NewMachine(vmmCtx, fcCfg, firecracker.WithLogger(log.NewEntry(logger)))
	if err != nil {
		panic(err)
	}

	if err := m.Start(vmmCtx); err != nil {
		panic(err)
	}
	defer m.StopVMM()

	// wait for the VMM to exit
	if err := m.Wait(vmmCtx); err != nil {
		panic(err)
	}
}
