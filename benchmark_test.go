package firecracker

import (
	"bufio"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

const numberOfVMs = 200

func createMachine(ctx context.Context, name string, forwardSignals []os.Signal) (*Machine, func(), error) {
	dir, err := ioutil.TempDir("", name)
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
			VcpuCount:   Int64(1),
			CPUTemplate: models.CPUTemplate(models.CPUTemplateT2),
			MemSizeMib:  Int64(256),
			HtEnabled:   Bool(false),
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
		var wg sync.WaitGroup
		for j := 0; j < numberOfVMs; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				machine, cleanup, err := createMachine(ctx, b.Name(), forwardSignals)
				if err != nil {
					b.Fatalf("failed to create a VM: %s", err)
				}
				defer cleanup()

				err = startAndWaitVM(ctx, machine)
				if err != nil && !strings.Contains(err.Error(), "signal: terminated") {
					b.Fatalf("failed to start the VM: %s", err)
				}
			}()
		}
		wg.Wait()
	}
}
func BenchmarkForwardSignalsDefault(t *testing.B) {
	benchmarkForwardSignals(t, nil)
}

func BenchmarkForwardSignalsDisable(t *testing.B) {
	benchmarkForwardSignals(t, []os.Signal{})
}
