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

package firecracker

import (
	"context"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	ops "github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	"github.com/firecracker-microvm/firecracker-go-sdk/fctesting"
	"github.com/stretchr/testify/assert"
)

const (
	firecrackerBinaryPath        = "firecracker"
	firecrackerBinaryOverrideEnv = "FC_TEST_BIN"

	defaultJailerBinary = "jailer"

	defaultTuntapName = "fc-test-tap0"
	tuntapOverrideEnv = "FC_TEST_TAP"

	testDataPathEnv = "FC_TEST_DATA_PATH"

	sudoUID = "SUDO_UID"
	sudoGID = "SUDO_GID"
)

var (
	skipTuntap   bool
	testDataPath = "./testdata"
)

func init() {
	flag.BoolVar(&skipTuntap, "test.skip-tuntap", false, "Disables tests that require a tuntap device")

	if val := os.Getenv(testDataPathEnv); val != "" {
		testDataPath = val
	}
}

// Ensure that we can create a new machine
func TestNewMachine(t *testing.T) {
	m, err := NewMachine(
		context.Background(),
		Config{
			Debug:             true,
			DisableValidation: true,
			MachineCfg: models.MachineConfiguration{
				VcpuCount:   Int64(1),
				MemSizeMib:  Int64(100),
				CPUTemplate: models.CPUTemplate(models.CPUTemplateT2),
				HtEnabled:   Bool(false),
			},
			JailerCfg: JailerConfig{
				GID:            Int(100),
				UID:            Int(100),
				ID:             "my-micro-vm",
				NumaNode:       Int(0),
				ExecFile:       "/path/to/firecracker",
				ChrootStrategy: NewNaiveChrootStrategy("path", "kernel-image-path"),
			},
		},
		WithLogger(fctesting.NewLogEntry()))
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	m.Handlers.Validation = m.Handlers.Validation.Clear()

	if m == nil {
		t.Errorf("NewMachine did not create a Machine")
	}
}

func TestJailerMicroVMExecution(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	fctesting.RequiresRoot(t)

	jailerUID := 123
	jailerGID := 100
	var err error
	if v := os.Getenv(sudoUID); v != "" {
		if jailerUID, err = strconv.Atoi(v); err != nil {
			t.Fatalf("Failed to parse %q", sudoUID)
		}
	}

	if v := os.Getenv(sudoGID); v != "" {
		if jailerGID, err = strconv.Atoi(v); err != nil {
			t.Fatalf("Failed to parse %q", sudoGID)
		}
	}

	// uses temp directory due to testdata's path being too long which causes a
	// SUN_LEN error.
	tmpDir, err := ioutil.TempDir(os.TempDir(), "jailer-test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}

	vmlinuxPath := filepath.Join(tmpDir, "vmlinux")
	if err := copyFile(filepath.Join(testDataPath, "vmlinux"), vmlinuxPath, jailerUID, jailerGID); err != nil {
		t.Fatalf("Failed to copy the vmlinux file: %v", err)
	}

	rootdrivePath := filepath.Join(tmpDir, "root-drive.img")
	if err := copyFile(filepath.Join(testDataPath, "root-drive.img"), rootdrivePath, jailerUID, jailerGID); err != nil {
		t.Fatalf("Failed to copy the root drive file: %v", err)
	}

	var nCpus int64 = 2
	cpuTemplate := models.CPUTemplate(models.CPUTemplateT2)
	var memSz int64 = 256

	// short names and directory to prevent SUN_LEN error
	id := "b"
	jailerTestPath := tmpDir
	jailerFullRootPath := filepath.Join(jailerTestPath, "firecracker", id)
	os.MkdirAll(jailerTestPath, 0777)

	socketPath := filepath.Join(jailerTestPath, "firecracker", "TestJailerMicroVMExecution.socket")
	logFifo := filepath.Join(tmpDir, "firecracker.log")
	metricsFifo := filepath.Join(tmpDir, "firecracker-metrics")
	defer func() {
		os.Remove(socketPath)
		os.Remove(logFifo)
		os.Remove(metricsFifo)
		os.RemoveAll(tmpDir)
	}()

	cfg := Config{
		Debug:           true,
		SocketPath:      socketPath,
		LogFifo:         logFifo,
		MetricsFifo:     metricsFifo,
		LogLevel:        "Debug",
		KernelImagePath: vmlinuxPath,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:   Int64(nCpus),
			CPUTemplate: cpuTemplate,
			MemSizeMib:  Int64(memSz),
			HtEnabled:   Bool(false),
		},
		Drives: []models.Drive{
			models.Drive{
				DriveID:      String("1"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(rootdrivePath),
			},
		},
		JailerCfg: JailerConfig{
			GID:            Int(jailerGID),
			UID:            Int(jailerUID),
			NumaNode:       Int(0),
			ID:             id,
			ChrootBaseDir:  jailerTestPath,
			ExecFile:       getFirecrackerBinaryPath(),
			ChrootStrategy: NewNaiveChrootStrategy(jailerFullRootPath, vmlinuxPath),
		},
		EnableJailer: true,
	}

	if _, err := os.Stat(vmlinuxPath); err != nil {
		t.Fatalf("Cannot find vmlinux file: %s\n"+
			`Verify that you have a vmlinux file at "%s" or set the `+
			"`%s` environment variable to the correct location.",
			err, vmlinuxPath, testDataPathEnv)
	}

	kernelImageInfo := syscall.Stat_t{}
	if err := syscall.Stat(cfg.KernelImagePath, &kernelImageInfo); err != nil {
		t.Fatalf("Failed to stat kernel image: %v", err)
	}

	if kernelImageInfo.Uid != uint32(jailerUID) || kernelImageInfo.Gid != uint32(jailerGID) {
		t.Fatalf("Kernel image does not have the proper UID or GID.\n"+
			"To fix this simply run:\n"+
			"sudo chown %d:%d %s",
			jailerUID, jailerGID, cfg.KernelImagePath)
	}

	for _, drive := range cfg.Drives {
		driveImageInfo := syscall.Stat_t{}
		drivePath := StringValue(drive.PathOnHost)
		if err := syscall.Stat(drivePath, &driveImageInfo); err != nil {
			t.Fatalf("Failed to stat kernel image: %v", err)
		}

		if driveImageInfo.Uid != uint32(jailerUID) || kernelImageInfo.Gid != uint32(jailerGID) {
			t.Fatalf("Drive does not have the proper UID or GID.\n"+
				"To fix this simply run:\n"+
				"sudo chown %d:%d %s",
				jailerUID, jailerGID, drivePath)
		}
	}

	jailerBin := defaultJailerBinary
	if _, err := os.Stat(filepath.Join(testDataPath, defaultJailerBinary)); err == nil {
		jailerBin = filepath.Join(testDataPath, defaultJailerBinary)
	}

	ctx := context.Background()
	cmd := NewJailerCommandBuilder().
		WithBin(jailerBin).
		WithGID(IntValue(cfg.JailerCfg.GID)).
		WithUID(IntValue(cfg.JailerCfg.UID)).
		WithNumaNode(IntValue(cfg.JailerCfg.NumaNode)).
		WithID(cfg.JailerCfg.ID).
		WithChrootBaseDir(cfg.JailerCfg.ChrootBaseDir).
		WithExecFile(cfg.JailerCfg.ExecFile).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		Build(ctx)

	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry()))
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	vmmCtx, vmmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer vmmCancel()

	if err := m.Start(vmmCtx); err != nil {
		t.Errorf("Failed to start VMM: %v", err)
	}

	m.StopVMM()
}

func TestMicroVMExecution(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	var nCpus int64 = 2
	cpuTemplate := models.CPUTemplate(models.CPUTemplateT2)
	var memSz int64 = 256
	socketPath := filepath.Join(testDataPath, "TestMicroVMExecution.sock")
	logFifo := filepath.Join(testDataPath, "firecracker.log")
	metricsFifo := filepath.Join(testDataPath, "firecracker-metrics")
	defer func() {
		os.Remove(socketPath)
		os.Remove(logFifo)
		os.Remove(metricsFifo)
	}()

	vmlinuxPath := getVmlinuxPath(t)

	networkIfaces := []NetworkInterface{
		{
			MacAddress:  "01-23-45-67-89-AB-CD-EF",
			HostDevName: "tap0",
		},
	}

	cfg := Config{
		SocketPath:  socketPath,
		LogFifo:     logFifo,
		MetricsFifo: metricsFifo,
		LogLevel:    "Debug",
		MachineCfg: models.MachineConfiguration{
			VcpuCount:   Int64(nCpus),
			CPUTemplate: cpuTemplate,
			MemSizeMib:  Int64(memSz),
			HtEnabled:   Bool(false),
		},
		Debug:             true,
		DisableValidation: true,
		NetworkInterfaces: networkIfaces,
	}

	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(socketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)

	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry()))
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	m.Handlers.Validation = m.Handlers.Validation.Clear()

	vmmCtx, vmmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer vmmCancel()
	exitchannel := make(chan error)
	go func() {
		if err := m.startVMM(vmmCtx); err != nil {
			close(exitchannel)
			t.Fatalf("Failed to start VMM: %v", err)
		}
		defer m.StopVMM()

		exitchannel <- m.Wait(vmmCtx)
		close(exitchannel)
	}()

	deadlineCtx, deadlineCancel := context.WithTimeout(vmmCtx, 250*time.Millisecond)
	defer deadlineCancel()
	if err := waitForAliveVMM(deadlineCtx, m.client); err != nil {
		t.Fatal(err)
	}

	t.Run("TestCreateMachine", func(t *testing.T) { testCreateMachine(ctx, t, m) })
	t.Run("TestMachineConfigApplication", func(t *testing.T) { testMachineConfigApplication(ctx, t, m, cfg) })
	t.Run("TestCreateBootSource", func(t *testing.T) { testCreateBootSource(ctx, t, m, vmlinuxPath) })
	t.Run("TestCreateNetworkInterface", func(t *testing.T) { testCreateNetworkInterfaceByID(ctx, t, m) })
	t.Run("TestAttachRootDrive", func(t *testing.T) { testAttachRootDrive(ctx, t, m) })
	t.Run("TestAttachSecondaryDrive", func(t *testing.T) { testAttachSecondaryDrive(ctx, t, m) })
	t.Run("TestAttachVsock", func(t *testing.T) { testAttachVsock(ctx, t, m) })
	t.Run("SetMetadata", func(t *testing.T) { testSetMetadata(ctx, t, m) })
	t.Run("TestUpdateGuestDrive", func(t *testing.T) { testUpdateGuestDrive(ctx, t, m) })
	t.Run("TestUpdateGuestNetworkInterface", func(t *testing.T) { testUpdateGuestNetworkInterface(ctx, t, m) })
	t.Run("TestStartInstance", func(t *testing.T) { testStartInstance(ctx, t, m) })

	// Let the VMM start and stabilize...
	timer := time.NewTimer(5 * time.Second)
	select {
	case <-timer.C:
		t.Run("TestShutdown", func(t *testing.T) { testShutdown(ctx, t, m) })
	case <-exitchannel:
		// if we've already exited, there's no use waiting for the timer
	}
	// unconditionally stop the VM here. TestShutdown may have triggered a shutdown, but if it
	// didn't for some reason, we still need to terminate it:
	m.StopVMM()
	m.Wait(vmmCtx)
}

func TestStartVMM(t *testing.T) {
	socketPath := filepath.Join("testdata", "TestStartVMM.sock")
	defer os.Remove(socketPath)
	cfg := Config{
		SocketPath: socketPath,
	}
	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(cfg.SocketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)
	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry()))
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	m.Handlers.Validation = m.Handlers.Validation.Clear()
	timeout, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	err = m.startVMM(timeout)
	if err != nil {
		t.Fatalf("startVMM failed: %s", err)
	}
	defer m.StopVMM()

	select {
	case <-timeout.Done():
		if timeout.Err() == context.DeadlineExceeded {
			t.Log("firecracker ran for 250ms")
		} else {
			t.Errorf("startVMM returned %s", m.Wait(ctx))
		}
	}

	// Make sure exitCh close
	_, closed := <-m.exitCh
	assert.False(t, closed)
}

func TestStartVMMOnce(t *testing.T) {
	socketPath := filepath.Join("testdata", "TestStartVMMOnce.sock")
	defer os.Remove(socketPath)

	cfg := Config{
		SocketPath:        socketPath,
		DisableValidation: true,
		KernelImagePath:   getVmlinuxPath(t),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:   Int64(1),
			MemSizeMib:  Int64(64),
			CPUTemplate: models.CPUTemplate(models.CPUTemplateT2),
			HtEnabled:   Bool(false),
		},
	}
	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(cfg.SocketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)
	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	timeout, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	err = m.Start(timeout)
	if err != nil {
		t.Fatalf("startVMM failed: %s", err)
	}
	defer m.StopVMM()
	err = m.Start(timeout)
	assert.Error(t, err, "should return an error when Start is called multiple times")
	assert.Equal(t, ErrAlreadyStarted, err, "should be ErrAlreadyStarted")

	select {
	case <-timeout.Done():
		if timeout.Err() == context.DeadlineExceeded {
			t.Log("firecracker ran for 250ms")
			t.Run("TestStopVMM", func(t *testing.T) { testStopVMM(ctx, t, m) })
		} else {
			t.Errorf("startVMM returned %s", m.Wait(ctx))
		}
	}

}

func getFirecrackerBinaryPath() string {
	if val := os.Getenv(firecrackerBinaryOverrideEnv); val != "" {
		return val
	}
	return filepath.Join(testDataPath, firecrackerBinaryPath)
}

func getVmlinuxPath(t *testing.T) string {
	t.Helper()
	vmlinuxPath := filepath.Join(testDataPath, "./vmlinux")
	if _, err := os.Stat(vmlinuxPath); err != nil {
		t.Fatalf("Cannot find vmlinux file: %s\n"+
			`Verify that you have a vmlinux file at "%s" or set the `+
			"`%s` environment variable to the correct location.",
			err, vmlinuxPath, testDataPathEnv)
	}
	return vmlinuxPath
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
	assert.Equal(t, expectedValues.MachineCfg.VcpuCount,
		m.machineConfig.VcpuCount, "CPU count should be equal")

	assert.Equal(t, expectedValues.MachineCfg.MemSizeMib, m.machineConfig.MemSizeMib, "memory...")
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

func testUpdateGuestDrive(ctx context.Context, t *testing.T, m *Machine) {
	path := filepath.Join(testDataPath, "drive-3.img")
	if err := m.UpdateGuestDrive(ctx, "2", path); err != nil {
		t.Errorf("unexpected error on swapping guest drive: %v", err)
	}
}

func testUpdateGuestNetworkInterface(ctx context.Context, t *testing.T, m *Machine) {
	rateLimitSet := RateLimiterSet{
		InRateLimiter: NewRateLimiter(
			TokenBucketBuilder{}.WithBucketSize(10).WithRefillDuration(10).Build(),
			TokenBucketBuilder{}.WithBucketSize(10).WithRefillDuration(10).Build(),
		),
	}
	if err := m.UpdateGuestNetworkInterfaceRateLimit(ctx, "1", rateLimitSet); err != nil {
		t.Fatalf("Failed to update the network interface %v", err)
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
	drive := models.Drive{
		DriveID:      String("0"),
		IsRootDevice: Bool(true),
		IsReadOnly:   Bool(true),
		PathOnHost:   String(filepath.Join(testDataPath, "root-drive.img")),
	}
	err := m.attachDrives(ctx, drive)
	if err != nil {
		t.Errorf("attaching root drive failed: %s", err)
	}
}

func testAttachSecondaryDrive(ctx context.Context, t *testing.T, m *Machine) {
	drive := models.Drive{
		DriveID:      String("2"),
		IsRootDevice: Bool(false),
		IsReadOnly:   Bool(true),
		PathOnHost:   String(filepath.Join(testDataPath, "drive-2.img")),
	}
	err := m.attachDrive(ctx, drive)
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

func testShutdown(ctx context.Context, t *testing.T, m *Machine) {
	err := m.Shutdown(ctx)
	if err != nil {
		t.Errorf("machine.Shutdown() failed: %s", err)
	}
}

func TestWaitForSocket(t *testing.T) {
	okClient := fctesting.MockClient{}
	errClient := fctesting.MockClient{
		GetMachineConfigurationFn: func(params *ops.GetMachineConfigurationParams) (*ops.GetMachineConfigurationOK, error) {
			return nil, errors.New("http error")
		},
	}

	// testWaitForSocket has three conditions that need testing:
	// 1. The expected file is created within the deadline and
	//    socket HTTP request succeeded
	// 2. The expected file is not created within the deadline
	// 3. The process responsible for creating the file exits
	//    (indicated by an error published to exitchan)
	filename := "./test-create-file"
	errchan := make(chan error)

	m := Machine{
		cfg:    Config{SocketPath: filename},
		logger: fctesting.NewLogEntry(),
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, err := os.Create(filename)
		if err != nil {
			t.Fatalf("Unable to create test file %s: %s", filename, err)
		}
	}()

	// Socket file created, HTTP request succeeded
	m.client = NewClient(filename, fctesting.NewLogEntry(), true, WithOpsClient(&okClient))
	if err := m.waitForSocket(500*time.Millisecond, errchan); err != nil {
		t.Errorf("waitForSocket returned unexpected error %s", err)
	}

	// Socket file exists, HTTP request failed
	m.client = NewClient(filename, fctesting.NewLogEntry(), true, WithOpsClient(&errClient))
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

func TestLogFiles(t *testing.T) {
	cfg := Config{
		Debug:           true,
		KernelImagePath: filepath.Join(testDataPath, "vmlinux"), SocketPath: filepath.Join(testDataPath, "socket-path"),
		Drives: []models.Drive{
			models.Drive{
				DriveID:      String("0"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(filepath.Join(testDataPath, "root-drive.img")),
			},
		},
		DisableValidation: true,
	}

	opClient := fctesting.MockClient{
		GetMachineConfigurationFn: func(params *ops.GetMachineConfigurationParams) (*ops.GetMachineConfigurationOK, error) {
			return &ops.GetMachineConfigurationOK{
				Payload: &models.MachineConfiguration{},
			}, nil
		},
	}
	ctx := context.Background()
	client := NewClient("socket-path", fctesting.NewLogEntry(), true, WithOpsClient(&opClient))

	stdoutPath := filepath.Join(testDataPath, "stdout.log")
	stderrPath := filepath.Join(testDataPath, "stderr.log")
	stdout, err := os.Create(stdoutPath)
	stderr, err := os.Create(stderrPath)

	fd, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		t.Fatalf("unexpected error during creation of unix socket: %v", err)
	}

	defer func() {
		fd.Close()
	}()

	defer func() {
		os.Remove(stdoutPath)
		os.Remove(stderrPath)
	}()

	cmd := exec.Command("ls")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	m, err := NewMachine(
		ctx,
		cfg,
		WithClient(client),
		WithProcessRunner(cmd),
		WithLogger(fctesting.NewLogEntry()),
	)
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}
	defer m.StopVMM()

	if err := m.Handlers.FcInit.Run(ctx, m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(stdoutPath); os.IsNotExist(err) {
		t.Errorf("expected log file to be present")

	}

	if _, err := os.Stat(stderrPath); os.IsNotExist(err) {
		t.Errorf("expected log file to be present")
	}
}

func TestCaptureFifoToFile(t *testing.T) {
	fifoPath := filepath.Join(testDataPath, "fifo")

	if err := syscall.Mkfifo(fifoPath, 0700); err != nil {
		t.Fatalf("Unexpected error during syscall.Mkfifo call: %v", err)
	}
	defer os.Remove(fifoPath)

	f, err := os.OpenFile(fifoPath, os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("Failed to open file, %q: %v", fifoPath, err)
	}

	f.Write([]byte("Hello world!"))
	defer f.Close()

	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		select {
		case <-t.C:
			f.Close()
		}
	}()

	fifoLogPath := fifoPath + ".log"
	fifo, err := os.OpenFile(fifoLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to create fifo file: %v", err)
	}

	if err := captureFifoToFile(fctesting.NewLogEntry(), fifoPath, fifo); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	defer os.Remove(fifoLogPath)

	if _, err := os.Stat(fifoLogPath); err != nil {
		t.Errorf("Failed to stat file: %v", err)
	}
}

func TestSocketPathSet(t *testing.T) {
	socketpath := "foo/bar"
	m, err := NewMachine(context.Background(), Config{SocketPath: socketpath})
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}

	found := false
	for i := 0; i < len(m.cmd.Args); i++ {
		if m.cmd.Args[i] != "--api-sock" {
			continue
		}

		found = true
		if m.cmd.Args[i+1] != socketpath {
			t.Errorf("Incorrect socket path: %v", m.cmd.Args[i+1])
		}
		break
	}

	if !found {
		t.Errorf("Failed to find socket path")
	}
}

func copyFile(src, dst string, uid, gid int) error {
	srcFd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFd.Close()

	dstFd, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFd.Close()

	if _, err = io.Copy(dstFd, srcFd); err != nil {
		return err
	}

	if err := os.Chown(dst, uid, gid); err != nil {
		return err
	}

	return nil
}

func TestPID(t *testing.T) {
	m := &Machine{}
	if _, err := m.PID(); err == nil {
		t.Errorf("expected an error, but received none")
	}

	var nCpus int64 = 2
	cpuTemplate := models.CPUTemplate(models.CPUTemplateT2)
	var memSz int64 = 256
	socketPath := filepath.Join(testDataPath, "TestPID.sock")
	defer os.Remove(socketPath)

	vmlinuxPath := getVmlinuxPath(t)

	cfg := Config{
		SocketPath:      socketPath,
		KernelImagePath: vmlinuxPath,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:   Int64(nCpus),
			CPUTemplate: cpuTemplate,
			MemSizeMib:  Int64(memSz),
			HtEnabled:   Bool(false),
		},
		Drives: []models.Drive{
			models.Drive{
				DriveID:      String("1"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(filepath.Join(testDataPath, "root-drive.img")),
			},
		},
		Debug:             true,
		DisableValidation: true,
	}

	m, err := NewMachine(context.Background(), cfg)
	if err != nil {
		t.Errorf("expected no error during create machine, but received %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Errorf("expected no error during Start, but received %v", err)
	}
	defer m.StopVMM()

	pid, err := m.PID()
	if err != nil {
		t.Errorf("unexpected error during PID call, %v", err)
	}

	if pid == 0 {
		t.Errorf("failed to retrieve correct PID")
	}

	if err := m.StopVMM(); err != nil {
		t.Errorf("expected clean kill, but received %v", err)
	}

	m.Wait(context.Background())

	if _, err := m.PID(); err == nil {
		t.Errorf("expected an error, but received none")
	}

}
