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
package firecracker

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

const numberOfVMs = 200

func createMachine(ctx context.Context, name string, forwardSignals []os.Signal) (*Machine, func(), error) {
	dir, err := os.MkdirTemp("", name)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		os.RemoveAll(dir)
	}

	socketPath := filepath.Join(dir, "api.sock")
	vmlinuxPath := filepath.Join(testDataPath, "./vmlinux")
	logFifo := filepath.Join(dir, "log.fifo")
	metrics := filepath.Join(dir, "metrics.fifo")

	config := Config{
		SocketPath:      socketPath,
		KernelImagePath: vmlinuxPath,
		LogFifo:         logFifo,
		MetricsFifo:     metrics,
		LogLevel:        "Info",
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(1),
			MemSizeMib: Int64(256),
			Smt:        Bool(false),
		},
		Drives: []models.Drive{
			{
				DriveID:      String("root"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(true),
				PathOnHost:   String(testRootfs),
			},
		},
		ForwardSignals: forwardSignals,
	}

	cmd := VMCommandBuilder{}.
		WithSocketPath(socketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)

	log := logrus.New()
	log.SetLevel(logrus.FatalLevel)
	machine, err := NewMachine(ctx, config, WithProcessRunner(cmd), WithLogger(logrus.NewEntry(log)))
	if err != nil {
		return nil, cleanup, err
	}

	return machine, cleanup, nil
}

func startAndWaitVM(ctx context.Context, m *Machine) error {
	err := m.Start(ctx)
	if err != nil {
		return err
	}

	file, err := os.Open(m.LogFile())
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Guest-boot-time") {
			break
		}
	}
	err = m.StopVMM()
	if err != nil {
		return err
	}

	err = m.Wait(ctx)
	if err != nil {
		return err
	}

	return nil
}

func benchmarkForwardSignals(b *testing.B, forwardSignals []os.Signal) {
	ctx := context.Background()

	b.Logf("%s: %d", b.Name(), b.N)

	for i := 0; i < b.N; i++ {
		errCh := make(chan error, numberOfVMs)
		for j := 0; j < numberOfVMs; j++ {
			go func() {
				var err error
				defer func() { errCh <- err }()

				machine, cleanup, err := createMachine(ctx, b.Name(), forwardSignals)
				if err != nil {
					err = fmt.Errorf("failed to create a VM: %v", err)
					return // anonymous defer func() will deliver the error
				}
				defer cleanup()

				err = startAndWaitVM(ctx, machine)
				if err != nil && !strings.Contains(err.Error(), "signal: terminated") {
					err = fmt.Errorf("failed to start the VM: %v", err)
					return // anonymous defer func() will deliver the error
				}
				return // anonymous defer func() will deliver this nil error
			}()
		}
		for k := 0; k < numberOfVMs; k++ {
			err := <-errCh
			if err != nil {
				b.Fatal(err)
			}
		}
		close(errCh)
	}
}
func BenchmarkForwardSignalsDefault(t *testing.B) {
	benchmarkForwardSignals(t, nil)
}

func BenchmarkForwardSignalsDisable(t *testing.B) {
	benchmarkForwardSignals(t, []os.Signal{})
}
