package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"testing"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	ops "github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	"github.com/firecracker-microvm/firecracker-go-sdk/fctesting"
	"github.com/golang/mock/gomock"
)

func TestGenerateLogFile(t *testing.T) {
	cases := []struct {
		name             string
		path             string
		expectedFilePath string
		expectedErr      bool
	}{
		{
			name:             "simple case",
			path:             "../..//testdata/foo",
			expectedFilePath: "../../testdata/foo",
		},
		{
			name:        "directory case",
			path:        "../../testdata",
			expectedErr: true,
		},
		{
			name:        "invalid directory case",
			path:        "../../testdata/foo/bar/baz",
			expectedErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := generateLogFile(c.path)
			if err != nil && !c.expectedErr {
				t.Errorf("expected no error, but received %v", err)
			}
			f.Close()

			if err == nil && c.expectedErr {
				t.Errorf("expected an error, but received none")
			}

			if c.expectedErr {
				return
			}

			if _, err := os.Stat(c.expectedFilePath); os.IsNotExist(err) {
				t.Errorf("expected file to exist")
			}

			os.Remove(c.expectedFilePath)
		})
	}
}

func TestLogFiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cfg := firecracker.Config{
		Debug:           true,
		Console:         consoleFile,
		KernelImagePath: "../../testdata/vmlinux",
		SocketPath:      "../../testdata/socket-path",
		RootDrive: firecracker.BlockDevice{
			HostPath: "../../testdata/root-drive.img",
			Mode:     "rw",
		},
		DisableValidation: true,
	}

	client := fctesting.NewMockFirecracker(ctrl)
	ctx := context.Background()
	client.
		EXPECT().
		PutMachineConfiguration(
			ctx,
			gomock.Any(),
		).
		AnyTimes().
		Return(&ops.PutMachineConfigurationNoContent{}, nil)

	client.
		EXPECT().
		PutGuestBootSource(
			ctx,
			gomock.Any(),
		).
		AnyTimes().
		Return(&ops.PutGuestBootSourceNoContent{}, nil)

	client.
		EXPECT().
		PutGuestDriveByID(
			ctx,
			gomock.Any(),
			gomock.Any(),
		).
		AnyTimes().
		Return(&ops.PutGuestDriveByIDNoContent{}, nil)

	client.EXPECT().GetMachineConfig().AnyTimes().Return(&ops.GetMachineConfigOK{
		Payload: &models.MachineConfiguration{},
	}, nil)

	stdoutPath := "../../testdata/stdout.log"
	stderrPath := "../../testdata/stderr.log"
	opts := options{
		FcConsole:           consoleFile,
		FcLogStdoutFilePath: stdoutPath,
		FcLogStderrFilePath: stderrPath,
	}

	fd, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		t.Fatalf("unexpected error during creation of unix socket: %v", err)
	}

	defer func() {
		fd.Close()
	}()

	_, closeFn, err := buildCommand(ctx, cfg.SocketPath, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer closeFn()
	defer func() {
		os.Remove(stdoutPath)
		os.Remove(stderrPath)
	}()

	cmd := exec.Command("ls")
	m, err := firecracker.NewMachine(cfg,
		firecracker.WithClient(client),
		firecracker.WithProcessRunner(cmd),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = m.Init(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(stdoutPath); os.IsNotExist(err) {
		t.Errorf("expected log file to be present")

	}

	if _, err := os.Stat(stderrPath); os.IsNotExist(err) {
		t.Errorf("expected log file to be present")
	}
}
