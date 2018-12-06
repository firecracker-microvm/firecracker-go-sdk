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

//go:generate mockgen -source=machine.go -destination=fctesting/machine_mock.go -package=fctesting

package firecracker

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	"github.com/firecracker-microvm/firecracker-go-sdk/fctesting"
	"github.com/golang/mock/gomock"
)

const (
	firecrackerBinaryPath        = "firecracker"
	firecrackerBinaryOverrideEnv = "FC_TEST_BIN"

	defaultTuntapName = "fc-test-tap0"
	tuntapOverrideEnv = "FC_TEST_TAP"

	testDataPathEnv = "FC_TEST_DATA_PATH"
)

var testDataPath = "./testdata"

var skipTuntap bool

func init() {
	flag.BoolVar(&skipTuntap, "test.skip-tuntap", false, "Disables tests that require a tuntap device")

	if val := os.Getenv(testDataPathEnv); val != "" {
		testDataPath = val
	}
}

// Ensure that we can create a new machine
func TestNewMachine(t *testing.T) {
	m, err := NewMachine(
		Config{
			Debug:             true,
			DisableValidation: true,
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m == nil {
		t.Errorf("NewMachine did not create a Machine")
	}
}

func TestMicroVMExecution(t *testing.T) {
	var nCpus int64 = 2
	cpuTemplate := CPUTemplate(CPUTemplateT2)
	var memSz int64 = 256
	socketPath := filepath.Join(testDataPath, "firecracker.sock")
	logFifo := filepath.Join(testDataPath, "firecracker.log")
	metricsFifo := filepath.Join(testDataPath, "firecracker-metrics")
	defer func() {
		os.Remove(socketPath)
		os.Remove(logFifo)
		os.Remove(metricsFifo)
	}()

	vmlinuxPath := filepath.Join(testDataPath, "./vmlinux")
	if _, err := os.Stat(vmlinuxPath); err != nil {
		t.Fatalf("Cannot find vmlinux file: %s\n"+
			`Verify that you have a vmlinux file at "%s" or set the `+
			"`%s` environment variable to the correct location.",
			err, vmlinuxPath, testDataPathEnv)
	}

	cfg := Config{
		SocketPath:        socketPath,
		LogFifo:           logFifo,
		MetricsFifo:       metricsFifo,
		LogLevel:          "Debug",
		CPUCount:          nCpus,
		CPUTemplate:       cpuTemplate,
		MemInMiB:          memSz,
		Debug:             true,
		DisableValidation: true,
	}

	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(socketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)

	m, err := NewMachine(cfg, WithProcessRunner(cmd))
	if err != nil {
		t.Fatalf("unexpectd error: %v", err)
	}

	vmmCtx, vmmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer vmmCancel()
	var exitchannel <-chan error
	go func() {
		var err error
		exitchannel, err = m.startVMM(vmmCtx)
		if err != nil {
			t.Fatalf("Failed to start VMM: %v", err)
		}
	}()
	time.Sleep(2 * time.Second)

	t.Run("TestCreateMachine", func(t *testing.T) { testCreateMachine(ctx, t, m) })
	t.Run("TestMachineConfigApplication", func(t *testing.T) { testMachineConfigApplication(ctx, t, m, cfg) })
	t.Run("TestCreateBootSource", func(t *testing.T) { testCreateBootSource(ctx, t, m, vmlinuxPath) })
	t.Run("TestCreateNetworkInterface", func(t *testing.T) { testCreateNetworkInterfaceByID(ctx, t, m) })
	t.Run("TestAttachRootDrive", func(t *testing.T) { testAttachRootDrive(ctx, t, m) })
	t.Run("TestAttachSecondaryDrive", func(t *testing.T) { testAttachSecondaryDrive(ctx, t, m) })
	t.Run("TestAttachVsock", func(t *testing.T) { testAttachVsock(ctx, t, m) })
	t.Run("SetMetadata", func(t *testing.T) { testSetMetadata(ctx, t, m) })
	t.Run("TestStartInstance", func(t *testing.T) { testStartInstance(vmmCtx, t, m) })

	// Let the VMM start and stabilize...
	time.Sleep(5 * time.Second)
	t.Run("TestStopVMM", func(t *testing.T) { testStopVMM(ctx, t, m) })
	<-exitchannel
}

func TestStartVMM(t *testing.T) {
	socketPath := filepath.Join("testdata", "fc-start-vmm-test.sock")
	defer os.Remove(socketPath)
	cfg := Config{
		SocketPath:        socketPath,
		DisableValidation: true,
	}
	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(cfg.SocketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)
	m, err := NewMachine(cfg, WithProcessRunner(cmd))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.StopVMM()

	timeout, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	errchan, err := m.startVMM(timeout)
	if err != nil {
		t.Errorf("startVMM failed: %s", err)
	} else {
		select {
		case <-timeout.Done():
			if timeout.Err() == context.DeadlineExceeded {
				t.Log("firecracker ran for 250ms")
			} else {
				t.Errorf("startVMM returned %s", <-errchan)
			}
		}
	}
}

func getFirecrackerBinaryPath() string {
	if val := os.Getenv(firecrackerBinaryOverrideEnv); val != "" {
		return val
	}
	return filepath.Join(testDataPath, firecrackerBinaryPath)
}

func testCreateMachine(ctx context.Context, t *testing.T, m *Machine) {
	err := m.createMachine(ctx)
	if err != nil {
		t.Errorf("createMachine said %s", err)
	} else {
		t.Log("firecracker created a machine")
	}
}

func testMachineConfigApplication(ctx context.Context, t *testing.T, m *Machine, expectedValues Config) {
	if m.machineConfig.VcpuCount != expectedValues.CPUCount {
		t.Errorf("Got unexpected number of VCPUs: Expected: %d, Actual: %d",
			expectedValues.CPUCount, m.machineConfig.VcpuCount)
	}
	if m.machineConfig.MemSizeMib != expectedValues.MemInMiB {
		t.Errorf("Got unexpected value for machine memory: expected: %d, Got %d",
			expectedValues.MemInMiB, m.machineConfig.MemSizeMib)
	}
}

func testCreateBootSource(ctx context.Context, t *testing.T, m *Machine, vmlinuxPath string) {
	// panic=0: This option disables reboot-on-panic behavior for the kernel. We
	//          use this option as we might run the tests without a real root
	//          filesystem available to the guest.
	// Kernel command-line options can be found in the kernel source tree at
	// Documentation/admin-guide/kernel-parameters.txt.
	err := m.createBootSource(ctx, vmlinuxPath, "ro console=ttyS0 noapic reboot=k panic=0 pci=off nomodules")
	if err != nil {
		t.Errorf("failed to create boot source: %s", err)
	}
}

func testCreateNetworkInterfaceByID(ctx context.Context, t *testing.T, m *Machine) {
	if skipTuntap {
		t.Skip("Skipping: tuntap tests explicitly disabled")
	}
	hostDevName := getTapName()
	iface := NetworkInterface{
		MacAddress:  "02:00:00:01:02:03",
		HostDevName: hostDevName,
	}
	err := m.createNetworkInterface(ctx, iface, 1)
	if err != nil {
		t.Errorf(`createNetworkInterface: %s
Do you have a tuntap device named %s?
Create one with `+"`sudo ip tuntap add %s mode tap user $UID`", err, hostDevName, hostDevName)
	}
}

func getTapName() string {
	if val := os.Getenv(tuntapOverrideEnv); val != "" {
		return val
	}
	return defaultTuntapName
}

func testAttachRootDrive(ctx context.Context, t *testing.T, m *Machine) {
	drive := BlockDevice{HostPath: filepath.Join(testDataPath, "root-drive.img"), Mode: "ro"}
	err := m.attachRootDrive(ctx, drive)
	if err != nil {
		t.Errorf("attaching root drive failed: %s", err)
	}
}

func testAttachSecondaryDrive(ctx context.Context, t *testing.T, m *Machine) {
	drive := BlockDevice{HostPath: filepath.Join(testDataPath, "drive-2.img"), Mode: "ro"}
	err := m.attachDrive(ctx, drive, 2, false)
	if err != nil {
		t.Errorf("attaching secondary drive failed: %s", err)
	}
}

func testAttachVsock(ctx context.Context, t *testing.T, m *Machine) {
	dev := VsockDevice{
		CID:  3,
		Path: "foo",
	}
	err := m.addVsock(ctx, dev)
	if err != nil {
		if badRequest, ok := err.(*operations.PutGuestVsockByIDBadRequest); ok &&
			strings.HasPrefix(badRequest.Payload.FaultMessage, "Invalid request method and/or path") {
			t.Errorf(`attaching vsock failed: %s
Does your Firecracker binary have vsock support?
Build one with vsock support by running `+"`cargo build --release --features vsock` from within the Firecracker repository.",
				badRequest.Payload.FaultMessage)
		} else {
			t.Errorf("attaching vsock failed: %s", err)
		}
	}
}

func testStartInstance(ctx context.Context, t *testing.T, m *Machine) {
	err := m.startInstance(ctx)
	if err != nil {
		if syncErr, ok := err.(*operations.CreateSyncActionDefault); ok &&
			strings.HasPrefix(syncErr.Payload.FaultMessage, "Cannot create vsock device") {
			t.Errorf(`startInstance: %s
Do you have permission to interact with /dev/vhost-vsock?
Grant yourself permission with `+"`sudo setfacl -m u:${USER}:rw /dev/vhost-vsock`", syncErr.Payload.FaultMessage)
		} else {
			t.Errorf("startInstance failed: %s", err)
		}
	}
}

func testStopVMM(ctx context.Context, t *testing.T, m *Machine) {
	err := m.StopVMM()
	if err != nil {
		t.Errorf("StopVMM failed: %s", err)
	}
}

func TestWaitForSocket(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	okClient := fctesting.NewMockFirecracker(ctrl)
	okClient.EXPECT().GetMachineConfig().AnyTimes().Return(nil, nil)

	errClient := fctesting.NewMockFirecracker(ctrl)
	errClient.EXPECT().GetMachineConfig().AnyTimes().Return(nil, errors.New("http error"))

	// testWaitForSocket has three conditions that need testing:
	// 1. The expected file is created within the deadline and
	//    socket HTTP request succeeded
	// 2. The expected file is not created within the deadline
	// 3. The process responsible for creating the file exits
	//    (indicated by an error published to exitchan)
	filename := "./test-create-file"
	errchan := make(chan error)

	m := Machine{
		cfg: Config{SocketPath: filename},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, err := os.Create(filename)
		if err != nil {
			t.Fatalf("Unable to create test file %s: %s", filename, err)
		}
	}()

	// Socket file created, HTTP request succeeded
	m.client = okClient
	if err := m.waitForSocket(500*time.Millisecond, errchan); err != nil {
		t.Errorf("waitForSocket returned unexpected error %s", err)
	}

	// Socket file exists, HTTP request failed
	m.client = errClient
	if err := m.waitForSocket(500*time.Millisecond, errchan); err != context.DeadlineExceeded {
		t.Error("waitforSocket did not return an expected timeout error")
	}

	os.Remove(filename)

	// No socket file
	if err := m.waitForSocket(100*time.Millisecond, errchan); err != context.DeadlineExceeded {
		t.Error("waitforSocket did not return an expected timeout error")
	}

	chanErr := errors.New("this is an expected error")
	go func() {
		time.Sleep(50 * time.Millisecond)
		errchan <- chanErr
	}()

	// Unexpected process exit
	if err := m.waitForSocket(100*time.Millisecond, errchan); err != chanErr {
		t.Error("waitForSocket did not properly detect program exit")
	}
}

func testSetMetadata(ctx context.Context, t *testing.T, m *Machine) {
	metadata := map[string]string{"key": "value"}
	err := m.SetMetadata(ctx, metadata)
	if err != nil {
		t.Errorf("failed to set metadata: %s", err)
	}
}
