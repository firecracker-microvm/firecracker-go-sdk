package firecracker_test

import (
	"context"
	"fmt"
	"os"

	"github.com/firecracker-microvm/firecracker-go-sdk"
)

func ExampleWithProcessRunner_logging() {
	const socketPath = "/tmp/firecracker.sock"
	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: "/path/to/kernel",
		RootDrive: firecracker.BlockDevice{
			HostPath: "/path/to/root/drive",
			Mode:     "rw",
		},
		CPUCount: 1,
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

	m, err := firecracker.NewMachine(cfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		panic(fmt.Errorf("failed to create new machine: %v", err))
	}

	defer os.Remove(cfg.SocketPath)

	ch, err := m.Init(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to initialize machine: %v", err))
	}

	if err := m.StartInstance(ctx); err != nil {
		panic(fmt.Errorf("Failed to start instance: %v", err))
	}

	// wait for VMM to execute
	<-ch
}
