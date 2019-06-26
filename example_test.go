package firecracker_test

import (
	"context"
	"fmt"
	"os"
	"time"

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
		JailerCfg: firecracker.JailerConfig{
			GID:      firecracker.Int(100),
			UID:      firecracker.Int(100),
			ID:       "my-micro-vm",
			NumaNode: firecracker.Int(0),
			ExecFile: "/path/to/firecracker",
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

	networkIfaces := []firecracker.NetworkInterface{
		{
			MacAddress:     "01-23-45-67-89-AB-CD-EF",
			HostDevName:    "tap-name",
			InRateLimiter:  inbound,
			OutRateLimiter: outbound,
		},
	}

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

func ExampleJailerCommandBuilder() {
	ctx := context.Background()
	// Creates a jailer command using the JailerCommandBuilder.
	b := firecracker.NewJailerCommandBuilder().
		WithID("my-test-id").
		WithUID(123).
		WithGID(100).
		WithNumaNode(0).
		WithExecFile("/usr/local/bin/firecracker").
		WithChrootBaseDir("/tmp").
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	const socketPath = "/tmp/firecracker/my-test-id/api.socket"

	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: "./vmlinux",
		Drives: []models.Drive{
			models.Drive{
				DriveID:      firecracker.String("1"),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
				PathOnHost:   firecracker.String("/path/to/root/drive"),
			},
		},
		MachineCfg: models.MachineConfiguration{
			VcpuCount: firecracker.Int64(1),
		},
		DisableValidation: true,
	}

	// Passes the custom jailer command into the constructor
	m, err := firecracker.NewMachine(ctx, cfg, firecracker.WithProcessRunner(b.Build(ctx)))
	if err != nil {
		panic(fmt.Errorf("failed to create new machine: %v", err))
	}

	// This does not copy any of the files over to the rootfs since a process
	// runner was specified. This examples assumes that the files have been
	// properly mounted.
	if err := m.Start(ctx); err != nil {
		panic(err)
	}

	tCtx, cancelFn := context.WithTimeout(ctx, time.Minute)
	defer cancelFn()
	m.Wait(tCtx)
}
