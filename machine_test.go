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
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/vishvananda/netns"

	"github.com/containerd/fifo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	ops "github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	"github.com/firecracker-microvm/firecracker-go-sdk/fctesting"
	"github.com/firecracker-microvm/firecracker-go-sdk/internal"
)

const (
	firecrackerBinaryPath        = "firecracker"
	firecrackerBinaryOverrideEnv = "FC_TEST_BIN"

	defaultJailerBinary     = "jailer"
	jailerBinaryOverrideEnv = "FC_TEST_JAILER_BIN"

	defaultTuntapName = "fc-test-tap0"
	tuntapOverrideEnv = "FC_TEST_TAP"

	testDataPathEnv = "FC_TEST_DATA_PATH"

	sudoUID = "SUDO_UID"
	sudoGID = "SUDO_GID"
)

var (
	skipTuntap      bool
	testDataPath    = envOrDefault(testDataPathEnv, "./testdata")
	testDataLogPath = filepath.Join(testDataPath, "logs")
	testDataBin     = filepath.Join(testDataPath, "bin")

	testRootfs        = filepath.Join(testDataPath, "root-drive.img")
	testRootfsWithSSH = filepath.Join(testDataPath, "root-drive-with-ssh.img")
	testSSHKey        = filepath.Join(testDataPath, "root-drive-ssh-key")

	testBalloonMemory            = int64(10)
	testBalloonNewMemory         = int64(6)
	testBalloonDeflateOnOom      = true
	testStatsPollingIntervals    = int64(1)
	testNewStatsPollingIntervals = int64(6)
)

func envOrDefault(k, empty string) string {
	value := os.Getenv(k)
	if value == "" {
		return empty
	}
	return value
}

// Replace filesystem-unsafe characters (such as /) which are often seen in Go's test names
var fsSafeTestName = strings.NewReplacer("/", "_")

func makeSocketPath(tb testing.TB) (string, func()) {
	tb.Helper()

	dir, err := os.MkdirTemp("", fsSafeTestName.Replace(tb.Name()))
	require.NoError(tb, err)

	return filepath.Join(dir, "fc.sock"), func() { os.RemoveAll(dir) }
}

func init() {
	flag.BoolVar(&skipTuntap, "test.skip-tuntap", false, "Disables tests that require a tuntap device")

	if err := os.MkdirAll(testDataLogPath, 0777); err != nil {
		panic(err)
	}
}

// Ensure that we can create a new machine
func TestNewMachine(t *testing.T) {
	m, err := NewMachine(
		context.Background(),
		Config{
			DisableValidation: true,
			MachineCfg: models.MachineConfiguration{
				VcpuCount:  Int64(1),
				MemSizeMib: Int64(100),
				Smt:        Bool(false),
			},
		},
		WithLogger(fctesting.NewLogEntry(t)))
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	m.Handlers.Validation = m.Handlers.Validation.Clear()

	if m == nil {
		t.Errorf("NewMachine did not create a Machine")
	}
}

func TestJailerMicroVMExecution(t *testing.T) {
	fctesting.RequiresKVM(t)
	fctesting.RequiresRoot(t)

	logPath := filepath.Join(testDataLogPath, "TestJailerMicroVMExecution")
	err := os.MkdirAll(logPath, 0777)
	require.NoError(t, err, "unable to create %s path", logPath)

	jailerUID := 123
	jailerGID := 100
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
	tmpDir, err := os.MkdirTemp(os.TempDir(), "jailer-test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}

	vmlinuxPath := filepath.Join(tmpDir, "vmlinux")
	if err := copyFile(filepath.Join(testDataPath, "vmlinux"), vmlinuxPath, jailerUID, jailerGID); err != nil {
		t.Fatalf("Failed to copy the vmlinux file: %v", err)
	}

	rootdrivePath := filepath.Join(tmpDir, "root-drive.img")
	if err := copyFile(testRootfs, rootdrivePath, jailerUID, jailerGID); err != nil {
		t.Fatalf("Failed to copy the root drive file: %v", err)
	}

	var nCpus int64 = 2
	var memSz int64 = 256

	// short names and directory to prevent SUN_LEN error
	id := "b"
	jailerTestPath := tmpDir
	os.MkdirAll(jailerTestPath, 0777)

	socketPath := "TestJailerMicroVMExecution.socket"
	logFifo := filepath.Join(tmpDir, "firecracker.log")
	metricsFifo := filepath.Join(tmpDir, "firecracker-metrics")
	capturedLog := filepath.Join(tmpDir, "writer.fifo")
	fw, err := os.OpenFile(capturedLog, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err, "failed to open fifo writer file")
	defer func() {
		fw.Close()
		exec.Command("cp", capturedLog, logPath).Run()
		os.Remove(capturedLog)
		os.Remove(filepath.Join(jailerTestPath, "firecracker", socketPath))
		os.Remove(logFifo)
		os.Remove(metricsFifo)
		os.RemoveAll(tmpDir)
	}()

	logFd, err := os.OpenFile(
		filepath.Join(logPath, "TestJailerMicroVMExecution.log"),
		os.O_CREATE|os.O_RDWR,
		0666)
	require.NoError(t, err, "failed to create log file")
	defer logFd.Close()

	cfg := Config{
		SocketPath:      socketPath,
		LogFifo:         logFifo,
		MetricsFifo:     metricsFifo,
		LogLevel:        "Debug",
		KernelImagePath: vmlinuxPath,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(nCpus),
			MemSizeMib: Int64(memSz),
			Smt:        Bool(false),
		},
		Drives: []models.Drive{
			{
				DriveID:      String("1"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(rootdrivePath),
			},
		},
		JailerCfg: &JailerConfig{
			JailerBinary:   getJailerBinaryPath(),
			GID:            Int(jailerGID),
			UID:            Int(jailerUID),
			NumaNode:       Int(0),
			ID:             id,
			ChrootBaseDir:  jailerTestPath,
			ExecFile:       getFirecrackerBinaryPath(),
			ChrootStrategy: NewNaiveChrootStrategy(vmlinuxPath),
			Stdout:         logFd,
			Stderr:         logFd,
			CgroupVersion:  "2",
		},
		FifoLogWriter: fw,
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

	ctx := context.Background()
	m, err := NewMachine(ctx, cfg, WithLogger(fctesting.NewLogEntry(t)))
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	vmmCtx, vmmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer vmmCancel()

	if err := m.Start(vmmCtx); err != nil {
		t.Errorf("Failed to start VMM: %v", err)
	}

	m.StopVMM()

	info, err := os.Stat(capturedLog)
	assert.NoError(t, err, "failed to stat captured log file")
	assert.NotEqual(t, 0, info.Size())
}

func TestMicroVMExecution(t *testing.T) {
	fctesting.RequiresKVM(t)

	var nCpus int64 = 2
	var memSz int64 = 256

	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	socketPath := filepath.Join(dir, "TestMicroVMExecution.sock")
	logFifo := filepath.Join(dir, "firecracker.log")
	metricsFifo := filepath.Join(dir, "firecracker-metrics")
	capturedLog := filepath.Join(dir, "writer.fifo")
	fw, err := os.OpenFile(capturedLog, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err, "failed to open fifo writer file")
	defer fw.Close()

	vmlinuxPath := getVmlinuxPath(t)

	networkIfaces := []NetworkInterface{{
		StaticConfiguration: &StaticNetworkConfiguration{
			MacAddress:  "01-23-45-67-89-AB-CD-EF",
			HostDevName: "tap0",
		},
	}}

	cfg := Config{
		SocketPath:  socketPath,
		LogFifo:     logFifo,
		MetricsFifo: metricsFifo,
		LogLevel:    "Debug",
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(nCpus),
			MemSizeMib: Int64(memSz),
			Smt:        Bool(false),
		},
		DisableValidation: true,
		NetworkInterfaces: networkIfaces,
		FifoLogWriter:     fw,
	}

	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(socketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)

	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry(t)))
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	m.Handlers.Validation = m.Handlers.Validation.Clear()

	vmmCtx, vmmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer vmmCancel()
	exitchannel := make(chan error)
	go func() {
		err := m.startVMM(vmmCtx)
		if err != nil {
			exitchannel <- err
			close(exitchannel)
			return
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
	t.Run("TestGetFirecrackerVersion", func(t *testing.T) { testGetFirecrackerVersion(ctx, t, m) })
	t.Run("TestMachineConfigApplication", func(t *testing.T) { testMachineConfigApplication(ctx, t, m, cfg) })
	t.Run("TestCreateBootSource", func(t *testing.T) { testCreateBootSource(ctx, t, m, vmlinuxPath) })
	t.Run("TestCreateNetworkInterface", func(t *testing.T) { testCreateNetworkInterfaceByID(ctx, t, m) })
	t.Run("TestAttachRootDrive", func(t *testing.T) { testAttachRootDrive(ctx, t, m) })
	t.Run("TestAttachSecondaryDrive", func(t *testing.T) { testAttachSecondaryDrive(ctx, t, m) })
	t.Run("TestAttachVsock", func(t *testing.T) { testAttachVsock(ctx, t, m) })
	t.Run("SetMetadata", func(t *testing.T) { testSetMetadata(ctx, t, m) })
	t.Run("UpdateMetadata", func(t *testing.T) { testUpdateMetadata(ctx, t, m) })
	t.Run("GetMetadata", func(t *testing.T) { testGetMetadata(ctx, t, m) })         // Should be after testSetMetadata and testUpdateMetadata
	t.Run("TestCreateBalloon", func(t *testing.T) { testCreateBalloon(ctx, t, m) }) // should be before the microVM is started(only pre-boot)
	t.Run("TestStartInstance", func(t *testing.T) { testStartInstance(ctx, t, m) })
	t.Run("TestGetInstanceInfo", func(t *testing.T) { testGetInstanceInfo(ctx, t, m) })
	t.Run("TestGetBalloonConfig", func(t *testing.T) { testGetBalloonConfig(ctx, t, m) }) // should be after testCreateBalloon
	t.Run("TestGetBalloonStats", func(t *testing.T) { testGetBalloonStats(ctx, t, m) })

	// Let the VMM start and stabilize...
	timer := time.NewTimer(5 * time.Second)
	select {
	case <-timer.C:
		t.Run("TestUpdateGuestDrive", func(t *testing.T) { testUpdateGuestDrive(ctx, t, m) })
		t.Run("TestUpdateGuestNetworkInterface", func(t *testing.T) { testUpdateGuestNetworkInterface(ctx, t, m) })
		t.Run("TestUpdateBalloon", func(t *testing.T) { testUpdateBalloon(ctx, t, m) })
		t.Run("TestUpdateBalloonStats", func(t *testing.T) { testUpdateBalloonStats(ctx, t, m) })
		t.Run("TestShutdown", func(t *testing.T) { testShutdown(ctx, t, m) })
	case <-exitchannel:
		// if we've already exited, there's no use waiting for the timer
	}
	// unconditionally stop the VM here. TestShutdown may have triggered a shutdown, but if it
	// didn't for some reason, we still need to terminate it:
	m.StopVMM()
	m.Wait(vmmCtx)

	info, err := os.Stat(capturedLog)
	assert.NoError(t, err, "failed to stat captured log file")
	assert.NotEqual(t, 0, info.Size())
}

func TestStartVMM(t *testing.T) {
	fctesting.RequiresKVM(t)

	socketPath, cleanup := makeSocketPath(t)
	defer cleanup()

	cfg := Config{
		SocketPath: socketPath,
	}
	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(cfg.SocketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)
	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry(t)))
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

func TestLogAndMetrics(t *testing.T) {
	if skipLogAndMetricsTest() {
		t.Skip()
	}
	fctesting.RequiresKVM(t)

	tests := []struct {
		logLevel string
		quiet    bool
	}{
		{logLevel: "", quiet: false},
		{logLevel: "Info", quiet: false},
		{logLevel: "Error", quiet: true},
	}
	for _, test := range tests {
		t.Run(test.logLevel, func(t *testing.T) {
			out := testLogAndMetrics(t, test.logLevel)
			if test.quiet {
				// Non-released versions have version strings like
				// "v0.26-wip-145-g6cbffe0d".
				assert.Regexp(t, `^Running Firecracker v\d+\.\d+[\.-]`, out)
				return
			}

			// By default, Firecracker's log level is Warn.
			logLevel := "WARN"
			if test.logLevel != "" {
				logLevel = strings.ToUpper(test.logLevel)
			}
			assert.Contains(t, out, ":"+logLevel+"]")
		})
	}
}

func skipLogAndMetricsTest() bool {
	// Firecracker logging behavior has changed after
	// https://github.com/firecracker-microvm/firecracker/pull/4047
	// This includes default log level being changed from WARN to INFO
	// TODO: Update this test once firecracker version is upgraded to >v1.5.0
	version, err := getFirecrackerVersion()
	if err != nil {
		return true
	}
	// match version 1.4.x
	pattern := `^1\.4\.\d+$`
	match, _ := regexp.MatchString(pattern, version)
	return !match
}

func testLogAndMetrics(t *testing.T, logLevel string) string {
	const vmID = "UserSuppliedVMID"

	dir, err := os.MkdirTemp("", strings.Replace(t.Name(), "/", "_", -1))
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	socketPath := filepath.Join(dir, "fc.sock")

	cfg := Config{
		VMID:              vmID,
		SocketPath:        socketPath,
		DisableValidation: true,
		KernelImagePath:   getVmlinuxPath(t),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(1),
			MemSizeMib: Int64(64),
			Smt:        Bool(false),
		},
		MetricsPath: filepath.Join(dir, "fc-metrics.out"),
		LogPath:     filepath.Join(dir, "fc.log"),
		LogLevel:    logLevel,
	}
	ctx := context.Background()
	cmd := configureBuilder(VMCommandBuilder{}.WithBin(getFirecrackerBinaryPath()), cfg).Build(ctx)
	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry(t)))
	require.NoError(t, err)

	timeout, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()

	err = m.Start(timeout)
	require.NoError(t, err)
	defer m.StopVMM()

	select {
	case <-timeout.Done():
		if timeout.Err() == context.DeadlineExceeded {
			t.Log("firecracker ran for 250ms")
			t.Run("TestStopVMM", func(t *testing.T) { testStopVMM(ctx, t, m) })
		} else {
			t.Errorf("startVMM returned %s", m.Wait(ctx))
		}
	}

	metrics, err := os.Stat(cfg.MetricsPath)
	require.NoError(t, err)
	assert.NotEqual(t, 0, metrics.Size())

	log, err := os.Stat(cfg.LogPath)
	require.NoError(t, err)
	assert.NotEqual(t, 0, log.Size())

	content, err := os.ReadFile(cfg.LogPath)
	require.NoError(t, err)
	return string(content)
}

func TestStartVMMOnce(t *testing.T) {
	fctesting.RequiresKVM(t)

	socketPath, cleanup := makeSocketPath(t)
	defer cleanup()

	cfg := Config{
		SocketPath:        socketPath,
		DisableValidation: true,
		KernelImagePath:   getVmlinuxPath(t),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(1),
			MemSizeMib: Int64(64),
			Smt:        Bool(false),
		},
	}
	if cpu_temp, err := internal.SupportCPUTemplate(); cpu_temp && err == nil {
		cfg.MachineCfg.CPUTemplate = models.CPUTemplate(models.CPUTemplateT2)
	}
	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(cfg.SocketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)
	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry(t)))
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

func getJailerBinaryPath() string {
	if val := os.Getenv(jailerBinaryOverrideEnv); val != "" {
		return val
	}
	return filepath.Join(testDataPath, defaultJailerBinary)
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

func parseVersionFromStdout(stdout []byte) (string, error) {
	pattern := regexp.MustCompile(`Firecracker v(?P<version>[0-9]+\.[0-9]+\.[0-9]+-?.*)`)
	groupNames := pattern.SubexpNames()
	matches := pattern.FindStringSubmatch(string(stdout))

	for i, name := range groupNames {
		if name == "version" {
			return matches[i], nil
		}
	}

	return "", fmt.Errorf("Unable to parse firecracker version from stdout (Output: %s)",
		stdout)
}

func getFirecrackerVersion() (string, error) {
	cmd := exec.Command(getFirecrackerBinaryPath(), "--version")
	stdout, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return parseVersionFromStdout(stdout)
}

func testGetFirecrackerVersion(ctx context.Context, t *testing.T, m *Machine) {
	version, err := m.GetFirecrackerVersion(ctx)

	if err != nil {
		t.Errorf("GetFirecrackerVersion: %v", err)
	}

	expectedVersion, err := getFirecrackerVersion()
	if err != nil {
		t.Errorf("GetFirecrackerVersion: %v", err)
	}

	assert.Equalf(t, expectedVersion, version,
		"GetFirecrackerVersion: Expected version %v, got version %v",
		expectedVersion, version)
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
	err := m.createBootSource(ctx, vmlinuxPath, "", "ro console=ttyS0 noapic reboot=k panic=0 pci=off nomodules")
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
		StaticConfiguration: &StaticNetworkConfiguration{
			MacAddress:  "02:00:00:01:02:03",
			HostDevName: hostDevName,
		},
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
		PathOnHost:   String(testRootfs),
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
	timestamp := strconv.Itoa(int(time.Now().UnixNano()))
	dev := VsockDevice{
		ID:   "1",
		CID:  3,
		Path: timestamp + ".vsock",
	}
	err := m.addVsock(ctx, dev)
	if err != nil {
		if badRequest, ok := err.(*ops.PutGuestVsockBadRequest); ok &&
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
		if syncErr, ok := err.(*ops.CreateSyncActionDefault); ok &&
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

func TestStopVMMCleanup(t *testing.T) {
	fctesting.RequiresKVM(t)
	fctesting.RequiresRoot(t)

	socketPath, cleanup := makeSocketPath(t)
	defer cleanup()

	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cniConfDir := filepath.Join(dir, "cni.conf")
	err = os.MkdirAll(cniConfDir, 0777)
	require.NoError(t, err)

	cniBinPath := []string{testDataBin}

	const networkName = "fcnet"
	const ifName = "veth0"

	networkMask := "/24"
	subnet := "10.168.0.0" + networkMask

	cniConfPath := fmt.Sprintf("%s/%s.conflist", cniConfDir, networkName)
	err = writeCNIConfWithHostLocalSubnet(cniConfPath, networkName, subnet)
	require.NoError(t, err)
	defer os.Remove(cniConfPath)

	networkInterface := NetworkInterface{
		CNIConfiguration: &CNIConfiguration{
			NetworkName: networkName,
			IfName:      ifName,
			ConfDir:     cniConfDir,
			BinPath:     cniBinPath,
			VMIfName:    "eth0",
		},
	}

	cfg := Config{
		SocketPath:        socketPath,
		DisableValidation: true,
		KernelImagePath:   getVmlinuxPath(t),
		NetworkInterfaces: []NetworkInterface{networkInterface},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(1),
			MemSizeMib: Int64(64),
			Smt:        Bool(false),
		},
	}
	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(cfg.SocketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)
	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry(t)))
	require.NoError(t, err)
	err = m.Start(ctx)
	require.NoError(t, err)
	err = m.stopVMM()
	require.NoError(t, err)
	_, err = netns.GetFromName(m.Cfg.VMID)
	require.Error(t, err)
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
	filename, cleanup := makeSocketPath(t)
	defer cleanup()
	errchan := make(chan error)

	m := Machine{
		Cfg:    Config{SocketPath: filename},
		logger: fctesting.NewLogEntry(t),
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, err := os.Create(filename)
		if err != nil {
			t.Errorf("Unable to create test file %s: %s", filename, err)
			return
		}
	}()

	// Socket file created, HTTP request succeeded
	m.client = NewClient(filename, fctesting.NewLogEntry(t), true, WithOpsClient(&okClient))
	if err := m.waitForSocket(500*time.Millisecond, errchan); err != nil {
		t.Errorf("waitForSocket returned unexpected error %s", err)
	}

	// Socket file exists, HTTP request failed
	m.client = NewClient(filename, fctesting.NewLogEntry(t), true, WithOpsClient(&errClient))
	if err := m.waitForSocket(500*time.Millisecond, errchan); err != context.DeadlineExceeded {
		t.Error("waitforSocket did not return an expected timeout error")
	}

	cleanup()

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

func TestMicroVMExecutionWithMmdsV2(t *testing.T) {
	fctesting.RequiresKVM(t)

	var nCpus int64 = 2
	var memSz int64 = 256

	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	socketPath := filepath.Join(dir, "TestMicroVMExecution.sock")
	logFifo := filepath.Join(dir, "firecracker.log")
	metricsFifo := filepath.Join(dir, "firecracker-metrics")
	capturedLog := filepath.Join(dir, "writer.fifo")
	fw, err := os.OpenFile(capturedLog, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err, "failed to open fifo writer file")
	defer fw.Close()

	vmlinuxPath := getVmlinuxPath(t)

	networkIfaces := []NetworkInterface{{
		StaticConfiguration: &StaticNetworkConfiguration{
			MacAddress:  "01-23-45-67-89-AB-CD-EF",
			HostDevName: "tap0",
		},
	}}

	cfg := Config{
		SocketPath:  socketPath,
		LogFifo:     logFifo,
		MetricsFifo: metricsFifo,
		LogLevel:    "Debug",
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(nCpus),
			MemSizeMib: Int64(memSz),
			Smt:        Bool(false),
		},
		DisableValidation: true,
		NetworkInterfaces: networkIfaces,
		FifoLogWriter:     fw,
		MmdsVersion:       MMDSv2,
	}

	ctx := context.Background()
	cmd := VMCommandBuilder{}.
		WithSocketPath(socketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)

	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry(t)))
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	m.Handlers.Validation = m.Handlers.Validation.Clear()

	vmmCtx, vmmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer vmmCancel()
	exitchannel := make(chan error)
	go func() {
		err := m.startVMM(vmmCtx)
		if err != nil {
			exitchannel <- err
			close(exitchannel)
			return
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
	t.Run("TestCreateBootSource", func(t *testing.T) { testCreateBootSource(ctx, t, m, vmlinuxPath) })
	t.Run("TestCreateNetworkInterface", func(t *testing.T) { testCreateNetworkInterfaceByID(ctx, t, m) })
	t.Run("TestAttachRootDrive", func(t *testing.T) { testAttachRootDrive(ctx, t, m) })
	t.Run("SetMetadata", func(t *testing.T) { testSetMetadata(ctx, t, m) })
	t.Run("UpdateMetadata", func(t *testing.T) { testUpdateMetadata(ctx, t, m) })
	t.Run("GetMetadata", func(t *testing.T) { testGetMetadata(ctx, t, m) }) // Should be after testSetMetadata and testUpdateMetadata

	// unconditionally stop the VM here. TestShutdown may have triggered a shutdown, but if it
	// didn't for some reason, we still need to terminate it:
	err = m.StopVMM()
	assert.NoError(t, err, "failed to stop VM")
}

func testSetMetadata(ctx context.Context, t *testing.T, m *Machine) {
	metadata := map[string]string{"key": "value"}
	err := m.SetMetadata(ctx, metadata)
	if err != nil {
		t.Errorf("failed to set metadata: %s", err)
	}
}

func testUpdateMetadata(ctx context.Context, t *testing.T, m *Machine) {
	metadata := map[string]string{"patch_key": "patch_value"}
	err := m.UpdateMetadata(ctx, metadata)
	if err != nil {
		t.Errorf("failed to set metadata: %s", err)
	}
}

func testGetMetadata(ctx context.Context, t *testing.T, m *Machine) {
	metadata := struct {
		Key      string `json:"key"`
		PatchKey string `json:"patch_key"`
	}{}
	if err := m.GetMetadata(ctx, &metadata); err != nil {
		t.Errorf("failed to get metadata: %s", err)
	}

	if metadata.Key != "value" || metadata.PatchKey != "patch_value" {
		t.Error("failed to get expected metadata values")
	}
}

func testGetInstanceInfo(ctx context.Context, t *testing.T, m *Machine) {
	instance, err := m.DescribeInstanceInfo(ctx)
	if err != nil {
		t.Error("failed to get instance info")
	}

	if instance.AppName == nil || *instance.AppName == "" {
		t.Error("Invalid instance App name")
	}

	if instance.ID == nil || *instance.ID == "" {
		t.Error("Invalid instance ID")
	}

	if instance.State == nil || *instance.State == "" {
		t.Error("Invalid instance state")
	}

	if instance.VmmVersion == nil || *instance.VmmVersion == "" {
		t.Error("Invalid VmmVersion")
	}
}

func TestLogFiles(t *testing.T) {
	cfg := Config{
		KernelImagePath: filepath.Join(testDataPath, "vmlinux"), SocketPath: filepath.Join(testDataPath, "socket-path"),
		Drives: []models.Drive{
			{
				DriveID:      String("0"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(testRootfs),
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
	client := NewClient("socket-path", fctesting.NewLogEntry(t), true, WithOpsClient(&opClient))

	stdoutPath := filepath.Join(testDataPath, "stdout.log")
	stderrPath := filepath.Join(testDataPath, "stderr.log")
	stdout, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatalf("error creating %q: %v", stdoutPath, err)
	}
	stderr, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("error creating %q: %v", stderrPath, err)
	}

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
		WithLogger(fctesting.NewLogEntry(t)),
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
	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	fifoPath := filepath.Join(dir, "TestCaptureFifoToFile")

	if err := syscall.Mkfifo(fifoPath, 0700); err != nil {
		t.Fatalf("Unexpected error during syscall.Mkfifo call: %v", err)
	}
	defer os.Remove(fifoPath)

	f, err := os.OpenFile(fifoPath, os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("Failed to open file, %q: %v", fifoPath, err)
	}

	expectedBytes := []byte("Hello world!")
	f.Write(expectedBytes)
	defer f.Close()

	time.AfterFunc(250*time.Millisecond, func() { f.Close() })

	logPath := fifoPath + ".log"
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	testWriter := &fctesting.TestWriter{
		WriteFn: func(b []byte) (int, error) {
			defer wg.Done()

			return logFile.Write(b)
		},
	}

	m := &Machine{
		exitCh: make(chan struct{}),
	}
	if err := m.captureFifoToFile(context.Background(), fctesting.NewLogEntry(t), fifoPath, testWriter); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	defer os.Remove(logPath)

	wg.Wait()
	_, err = os.Stat(logPath)
	assert.NoError(t, err, "failed to stat file")
	b, err := os.ReadFile(logPath)
	assert.NoError(t, err, "failed to read logPath")
	assert.Equal(t, expectedBytes, b)
}

func TestCaptureFifoToFile_nonblock(t *testing.T) {
	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	fifoPath := filepath.Join(dir, "TestCaptureFifoToFile_nonblock")

	if err := syscall.Mkfifo(fifoPath, 0700); err != nil {
		t.Fatalf("Unexpected error during syscall.Mkfifo call: %v", err)
	}
	defer os.Remove(fifoPath)

	logPath := fifoPath + ".log"
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	testWriter := &fctesting.TestWriter{
		WriteFn: func(b []byte) (int, error) {
			defer wg.Done()

			return logFile.Write(b)
		},
	}

	m := &Machine{
		exitCh: make(chan struct{}),
	}
	if err := m.captureFifoToFile(context.Background(), fctesting.NewLogEntry(t), fifoPath, testWriter); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	defer os.Remove(logPath)

	// we sleep here to check to see the io.Copy is working properly in
	// captureFifoToFile. This is due to the fifo being opened with O_NONBLOCK,
	// which causes io.Copy to exit immediately with no error.
	//
	// https://github.com/firecracker-microvm/firecracker-go-sdk/issues/156
	time.Sleep(250 * time.Millisecond)

	f, err := os.OpenFile(fifoPath, os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("Failed to open file, %q: %v", fifoPath, err)
	}
	expectedBytes := []byte("Hello world!")
	f.Write(expectedBytes)
	defer f.Close()

	time.AfterFunc(250*time.Millisecond, func() { f.Close() })

	wg.Wait()
	_, err = os.Stat(logPath)
	assert.NoError(t, err, "failed to stat file")
	b, err := os.ReadFile(logPath)
	assert.NoError(t, err, "failed to read logPath")
	assert.Equal(t, expectedBytes, b)
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
	if testing.Short() {
		t.Skip()
	}
	fctesting.RequiresRoot(t)

	m := &Machine{}
	if _, err := m.PID(); err == nil {
		t.Errorf("expected an error, but received none")
	}

	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	var nCpus int64 = 2
	var memSz int64 = 256
	socketPath := filepath.Join(dir, "TestPID.sock")
	defer os.Remove(socketPath)

	vmlinuxPath := getVmlinuxPath(t)

	rootfsBytes, err := os.ReadFile(testRootfs)
	require.NoError(t, err, "failed to read rootfs file")
	rootfsPath := filepath.Join(dir, "TestPID.img")
	err = os.WriteFile(rootfsPath, rootfsBytes, 0666)
	require.NoError(t, err, "failed to copy vm rootfs to %s", rootfsPath)

	cfg := Config{
		SocketPath:      socketPath,
		KernelImagePath: vmlinuxPath,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(nCpus),
			MemSizeMib: Int64(memSz),
			Smt:        Bool(false),
		},
		Drives: []models.Drive{
			{
				DriveID:      String("1"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(rootfsPath),
			},
		},
		DisableValidation: true,
	}

	ctx := context.Background()

	cmd := VMCommandBuilder{}.
		WithSocketPath(cfg.SocketPath).
		WithBin(getFirecrackerBinaryPath()).
		Build(ctx)

	m, err = NewMachine(ctx, cfg, WithProcessRunner(cmd), WithLogger(fctesting.NewLogEntry(t)))
	if err != nil {
		t.Errorf("expected no error during create machine, but received %v", err)
	}

	if err := m.Start(ctx); err != nil {
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

	m.Wait(ctx)

	if _, err := m.PID(); err == nil {
		t.Errorf("expected an error, but received none")
	}

}

func TestCaptureFifoToFile_leak(t *testing.T) {
	m := &Machine{
		exitCh: make(chan struct{}),
	}

	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	fifoPath := filepath.Join(dir, "TestCaptureFifoToFileLeak.fifo")
	err = syscall.Mkfifo(fifoPath, 0700)
	require.NoError(t, err, "failed to make fifo")
	defer os.Remove(fifoPath)

	fd, err := syscall.Open(fifoPath, syscall.O_RDWR|syscall.O_NONBLOCK, 0600)
	require.NoError(t, err, "failed to open fifo path")
	f := os.NewFile(uintptr(fd), fifoPath)
	assert.NotNil(t, f, "failed to create new  file")
	go func() {
		for {
			select {
			case <-m.exitCh:
				break
			default:
				_, err := f.Write([]byte("A"))
				assert.NoError(t, err, "failed to write bytes to fifo")
			}
		}
	}()

	buf := bytes.NewBuffer(nil)

	loggerBuffer := bytes.NewBuffer(nil)
	logger := fctesting.NewLogEntry(t)
	logger.Logger.Level = logrus.WarnLevel
	logger.Logger.Out = loggerBuffer

	done := make(chan error)
	err = m.captureFifoToFileWithChannel(context.Background(), logger, fifoPath, buf, done)
	assert.NoError(t, err, "failed to capture fifo to file")

	// Stopping the machine will close the FIFO
	close(m.exitCh)

	// Waiting the channel to make sure that the contents of the FIFO has been copied
	copyErr := <-done

	if copyErr == fifo.ErrReadClosed {
		// If the fifo package is aware about that the fifo is closed, we can get the error below.
		assert.Contains(t, loggerBuffer.String(), fifo.ErrReadClosed.Error(), "log")
	} else {
		// If not, the error would be something like "read /proc/self/fd/9: file already closed"
		assert.Contains(t, copyErr.Error(), `file already closed`, "error")
		assert.Contains(t, loggerBuffer.String(), `file already closed`, "log")
	}
}

func TestWait(t *testing.T) {
	fctesting.RequiresKVM(t)
	fctesting.RequiresRoot(t)

	cases := []struct {
		name string
		stop func(m *Machine, cancel context.CancelFunc)
	}{
		{
			name: "StopVMM",
			stop: func(m *Machine, _ context.CancelFunc) {
				err := m.StopVMM()
				require.NoError(t, err)
			},
		},
		{
			name: "Kill",
			stop: func(m *Machine, cancel context.CancelFunc) {
				pid, err := m.PID()
				require.NoError(t, err)

				process, err := os.FindProcess(pid)
				require.NoError(t, err)
				err = process.Kill()
				require.NoError(t, err)
			},
		},
		{
			name: "Context Cancel",
			stop: func(m *Machine, cancel context.CancelFunc) {
				cancel()
			},
		},
		{
			name: "StopVMM + Context Cancel",
			stop: func(m *Machine, cancel context.CancelFunc) {
				m.StopVMM()
				time.Sleep(1 * time.Second)
				cancel()
				time.Sleep(1 * time.Second)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			vmContext, vmCancel := context.WithCancel(context.Background())

			socketPath, cleanup := makeSocketPath(t)
			defer cleanup()

			// Tee logs for validation:
			var logBuffer bytes.Buffer
			machineLogger := logrus.New()
			machineLogger.Out = io.MultiWriter(os.Stderr, &logBuffer)

			cfg := createValidConfig(t, socketPath)
			m, err := NewMachine(ctx, cfg, func(m *Machine) {
				// Rewriting m.cmd partially wouldn't work since Cmd has
				// some unexported members
				args := m.cmd.Args[1:]
				m.cmd = exec.Command(getFirecrackerBinaryPath(), args...)
			}, WithLogger(logrus.NewEntry(machineLogger)))
			require.NoError(t, err)

			err = m.Start(vmContext)
			require.NoError(t, err)

			pid, err := m.PID()
			require.NoError(t, err)

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.stop(m, vmCancel)
			}()

			err = m.Wait(ctx)
			require.Error(t, err, "Firecracker was killed and it must be reported")
			t.Logf("err = %v", err)

			proc, err := os.FindProcess(pid)
			// Having an error here doesn't mean the process is not there.
			// In fact it won't be non-nil on Unix systems
			require.NoError(t, err)

			err = proc.Signal(syscall.Signal(0))
			require.Equal(t, "os: process already finished", err.Error())

			wg.Wait()

			machineLogs := logBuffer.String()
			assert.NotContains(t, machineLogs, "level=error")
			assert.NotContains(t, machineLogs, "process already finished")
		})
	}
}

func TestWaitWithInvalidBinary(t *testing.T) {
	ctx := context.Background()

	socketPath := filepath.Join(testDataPath, t.Name())
	defer os.Remove(socketPath)

	cfg := createValidConfig(t, socketPath)
	cmd := VMCommandBuilder{}.
		WithSocketPath(socketPath).
		WithBin("invalid-bin").
		Build(ctx)
	m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd))
	require.NoError(t, err)

	ch := make(chan error)

	go func() {
		err := m.Wait(ctx)
		require.Error(t, err, "Wait() reports an error")
		ch <- err
	}()

	err = m.Start(ctx)
	require.Error(t, err, "Start() reports an error")

	select {
	case errFromWait := <-ch:
		require.Equal(t, errFromWait, err)
	}
}

func TestWaitWithNoSocket(t *testing.T) {
	ctx := context.Background()

	socketPath := filepath.Join(testDataPath, t.Name())
	defer os.Remove(socketPath)
	cfg := createValidConfig(t, socketPath)

	m, err := NewMachine(ctx, cfg, WithProcessRunner(exec.Command("sleep", "10")))
	require.NoError(t, err)

	ch := make(chan error)

	go func() {
		err := m.Wait(ctx)
		require.Error(t, err, "Wait() reports an error")
		ch <- err
	}()

	err = m.Start(ctx)
	require.Error(t, err, "Start() reports an error")

	select {
	case errFromWait := <-ch:
		require.Equal(t, errFromWait, err)
	}
}

type machineConfigOpt func(c *Config)

func withRootDrive(rootfs string) machineConfigOpt {
	return func(c *Config) {
		var drives []models.Drive

		inserted := false
		for _, drive := range c.Drives {
			if *drive.IsRootDevice {
				drives = append(drives, models.Drive{
					DriveID:      String("root"),
					IsRootDevice: Bool(true),
					IsReadOnly:   Bool(false),
					PathOnHost:   String(rootfs),
				})
				inserted = true
			} else {
				drives = append(drives, drive)
			}
		}

		if !inserted {
			drives = append(drives, models.Drive{
				DriveID:      String("root"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(rootfs),
			})
		}

		c.Drives = drives
	}
}

func withNetworkInterface(networkInterface NetworkInterface) machineConfigOpt {
	return func(c *Config) {
		c.NetworkInterfaces = append(c.NetworkInterfaces, networkInterface)
	}
}

func createValidConfig(t *testing.T, socketPath string, opts ...machineConfigOpt) Config {
	cfg := Config{
		SocketPath:      socketPath,
		KernelImagePath: getVmlinuxPath(t),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  Int64(2),
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
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

func TestSignalForwarding(t *testing.T) {
	socketPath, cleanup := makeSocketPath(t)
	defer cleanup()

	forwardedSignals := []os.Signal{
		syscall.SIGUSR1,
		syscall.SIGUSR2,
		syscall.SIGINT,
		syscall.SIGTERM,
	}
	ignoredSignals := []os.Signal{
		syscall.SIGHUP,
		syscall.SIGQUIT,
	}

	cfg := Config{
		KernelImagePath: filepath.Join(testDataPath, "vmlinux"),
		SocketPath:      socketPath,
		Drives: []models.Drive{
			{
				DriveID:      String("0"),
				IsRootDevice: Bool(true),
				IsReadOnly:   Bool(false),
				PathOnHost:   String(testRootfs),
			},
		},
		DisableValidation: true,
		ForwardSignals:    forwardedSignals,
	}
	defer os.RemoveAll(socketPath)

	opClient := fctesting.MockClient{}

	ctx := context.Background()
	client := NewClient(cfg.SocketPath, fctesting.NewLogEntry(t), true, WithOpsClient(&opClient))

	fd, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		t.Fatalf("unexpected error during creation of unix socket: %v", err)
	}
	defer fd.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := exec.Command(filepath.Join(testDataPath, "sigprint.sh"))
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	assert.NoError(t, err)

	m, err := NewMachine(
		ctx,
		cfg,
		WithClient(client),
		WithProcessRunner(cmd),
		WithLogger(fctesting.NewLogEntry(t)),
	)
	if err != nil {
		t.Fatalf("failed to create new machine: %v", err)
	}

	if err := m.startVMM(ctx); err != nil {
		t.Fatalf("error startVMM: %v", err)
	}
	defer m.StopVMM()

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, ignoredSignals...)
	defer func() {
		signal.Stop(sigChan)
		close(sigChan)
	}()

	go func() {
		for sig := range sigChan {
			t.Logf("received signal %v, ignoring", sig)
		}
	}()

	go func() {
		for _, sig := range append(forwardedSignals, ignoredSignals...) {
			t.Logf("sending signal %v to self", sig)
			syscall.Kill(syscall.Getpid(), sig.(syscall.Signal))
		}

		// give the child process time to receive signals and flush pipes
		time.Sleep(1 * time.Second)

		// terminate the signal printing process
		stdin.Write([]byte("q"))
	}()

	err = m.Wait(ctx)
	require.NoError(t, err, "wait returned an error")

	receivedSignals := []os.Signal{}
	for _, sigStr := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		i, err := strconv.Atoi(sigStr)
		require.NoError(t, err, "expected numeric output")
		receivedSignals = append(receivedSignals, syscall.Signal(i))
	}

	assert.ElementsMatch(t, forwardedSignals, receivedSignals)
}

func TestPauseResume(t *testing.T) {
	fctesting.RequiresKVM(t)
	fctesting.RequiresRoot(t)

	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cases := []struct {
		name  string
		state func(m *Machine, ctx context.Context)
	}{
		{
			name: "PauseVM",
			state: func(m *Machine, ctx context.Context) {
				err := m.PauseVM(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "ResumeVM",
			state: func(m *Machine, ctx context.Context) {
				err := m.ResumeVM(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "Consecutive PauseVM",
			state: func(m *Machine, ctx context.Context) {
				err := m.PauseVM(ctx)
				require.NoError(t, err)

				err = m.PauseVM(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "Consecutive ResumeVM",
			state: func(m *Machine, ctx context.Context) {
				err := m.ResumeVM(ctx)
				require.NoError(t, err)

				err = m.ResumeVM(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "ResumeVM PauseVM",
			state: func(m *Machine, ctx context.Context) {
				err := m.ResumeVM(ctx)
				require.NoError(t, err)

				err = m.PauseVM(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "PauseVM ResumeVM",
			state: func(m *Machine, ctx context.Context) {
				err := m.PauseVM(ctx)
				require.NoError(t, err)

				err = m.ResumeVM(ctx)
				require.NoError(t, err)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()

			socketPath, cleanup := makeSocketPath(t)
			defer cleanup()

			// Tee logs for validation:
			var logBuffer bytes.Buffer
			machineLogger := logrus.New()
			machineLogger.Out = io.MultiWriter(os.Stderr, &logBuffer)

			cfg := createValidConfig(t, socketPath)
			m, err := NewMachine(ctx, cfg, func(m *Machine) {
				// Rewriting m.cmd partially wouldn't work since Cmd has
				// some unexported members
				args := m.cmd.Args[1:]
				m.cmd = exec.Command(getFirecrackerBinaryPath(), args...)
			}, WithLogger(logrus.NewEntry(machineLogger)))
			require.NoError(t, err)

			err = m.PauseVM(ctx)
			require.Error(t, err, "PauseVM must fail before Start is called")

			err = m.ResumeVM(ctx)
			require.Error(t, err, "ResumeVM must fail before Start is called")

			err = m.Start(ctx)
			require.NoError(t, err)

			c.state(m, ctx)

			err = m.StopVMM()
			require.NoError(t, err)

			err = m.PauseVM(ctx)
			require.Error(t, err, "PauseVM must fail after StopVMM is called")

			err = m.ResumeVM(ctx)
			require.Error(t, err, "ResumeVM must fail after StopVMM is called")
		})
	}
}

func TestCreateSnapshot(t *testing.T) {
	fctesting.RequiresKVM(t)
	fctesting.RequiresRoot(t)

	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cases := []struct {
		name           string
		createSnapshot func(m *Machine, ctx context.Context, memPath, snapPath string)
	}{
		{
			name: "CreateSnapshot",
			createSnapshot: func(m *Machine, ctx context.Context, memPath, snapPath string) {
				err := m.PauseVM(ctx)
				require.NoError(t, err)

				err = m.CreateSnapshot(ctx, memPath, snapPath)
				require.NoError(t, err)
			},
		},
		{
			name: "CreateSnapshot before pause",
			createSnapshot: func(m *Machine, ctx context.Context, memPath, snapPath string) {
				err := m.CreateSnapshot(ctx, memPath, snapPath)
				require.Error(t, err)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()

			socketPath := filepath.Join(dir, fsSafeTestName.Replace(t.Name()))
			snapPath := socketPath + "SnapFile"
			memPath := socketPath + "MemFile"
			defer os.Remove(socketPath)
			defer os.Remove(snapPath)
			defer os.Remove(memPath)

			// Tee logs for validation:
			var logBuffer bytes.Buffer
			machineLogger := logrus.New()
			machineLogger.Out = io.MultiWriter(os.Stderr, &logBuffer)

			cfg := createValidConfig(t, socketPath)
			m, err := NewMachine(ctx, cfg, func(m *Machine) {
				// Rewriting m.cmd partially wouldn't work since Cmd has
				// some unexported members
				args := m.cmd.Args[1:]
				m.cmd = exec.Command(getFirecrackerBinaryPath(), args...)
			}, WithLogger(logrus.NewEntry(machineLogger)))
			require.NoError(t, err)

			err = m.Start(ctx)
			require.NoError(t, err)

			c.createSnapshot(m, ctx, memPath, snapPath)

			err = m.StopVMM()
			require.NoError(t, err)
		})
	}
}

func connectToVM(m *Machine, sshKeyPath string) (*ssh.Client, error) {
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

	ip := m.Cfg.NetworkInterfaces.staticIPInterface().StaticConfiguration.IPConfiguration.IPAddr.IP

	return ssh.Dial("tcp", fmt.Sprintf("%s:22", ip), config)
}

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

func TestLoadSnapshot(t *testing.T) {
	fctesting.RequiresKVM(t)
	fctesting.RequiresRoot(t)

	dir, err := os.MkdirTemp("", t.Name())
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cniConfDir := filepath.Join(dir, "cni.conf")
	err = os.MkdirAll(cniConfDir, 0777)
	require.NoError(t, err)

	cniBinPath := []string{testDataBin}

	rootfsBytes, err := os.ReadFile(testRootfsWithSSH)
	require.NoError(t, err)
	rootfsPath := filepath.Join(dir, "rootfs.img")
	err = os.WriteFile(rootfsPath, rootfsBytes, 0666)
	require.NoError(t, err)

	sshKeyBytes, err := os.ReadFile(testSSHKey)
	require.NoError(t, err)
	sshKeyPath := filepath.Join(dir, "id_rsa")
	err = os.WriteFile(sshKeyPath, sshKeyBytes, 0600)
	require.NoError(t, err)

	// Using default cache directory to ensure collision avoidance on IP allocations
	const cniCacheDir = "/var/lib/cni"
	const networkName = "fcnet"
	const ifName = "veth0"

	networkMask := "/24"
	subnet := "10.168.0.0" + networkMask
	var ipToRestore string

	var maxRetries int = 20
	var backoffTimeMs time.Duration = 500

	cases := []struct {
		name           string
		createSnapshot func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string)
		loadSnapshot   func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string)
	}{
		{
			name: "TestLoadSnapshot",
			createSnapshot: func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string) {
				// Create a snapshot
				cfg := createValidConfig(t, socketPath+".create")
				m, err := NewMachine(ctx, cfg, func(m *Machine) {
					// Rewriting m.cmd partially wouldn't work since Cmd has
					// some unexported members
					args := m.cmd.Args[1:]
					m.cmd = exec.Command(getFirecrackerBinaryPath(), args...)
				}, WithLogger(logrus.NewEntry(machineLogger)))
				require.NoError(t, err)

				err = m.Start(ctx)
				require.NoError(t, err)

				err = m.PauseVM(ctx)
				require.NoError(t, err)

				err = m.CreateSnapshot(ctx, memPath, snapPath)
				require.NoError(t, err)

				err = m.StopVMM()
				require.NoError(t, err)
			},

			loadSnapshot: func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string) {
				// Note that many fields are not necessary when loading a snapshot
				cfg := Config{
					SocketPath: socketPath + ".load",
					Drives: []models.Drive{
						{
							DriveID:      String("root"),
							IsRootDevice: Bool(true),
							IsReadOnly:   Bool(true),
							PathOnHost:   String(testRootfs),
						},
					},
				}

				cfg.Snapshot.ResumeVM = false

				m, err := NewMachine(ctx, cfg, func(m *Machine) {
					// Rewriting m.cmd partially wouldn't work since Cmd has
					// some unexported members
					args := m.cmd.Args[1:]
					m.cmd = exec.Command(getFirecrackerBinaryPath(), args...)
				}, WithLogger(logrus.NewEntry(machineLogger)), WithSnapshot(memPath, snapPath, func(config *SnapshotConfig) {
					config.ResumeVM = true
				}))
				require.NoError(t, err)
				require.Equal(t, m.Cfg.Snapshot.ResumeVM, true)

				err = m.Start(ctx)
				require.NoError(t, err)

				err = m.StopVMM()
				require.NoError(t, err)
			},
		},
		{
			name: "TestLoadSnapshotWithMemoryBackend",
			createSnapshot: func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string) {
				// Create a snapshot
				cfg := createValidConfig(t, socketPath+".create")
				m, err := NewMachine(ctx, cfg, func(m *Machine) {
					// Rewriting m.cmd partially wouldn't work since Cmd has
					// some unexported members
					args := m.cmd.Args[1:]
					m.cmd = exec.Command(getFirecrackerBinaryPath(), args...)
				}, WithLogger(logrus.NewEntry(machineLogger)))
				require.NoError(t, err)

				err = m.Start(ctx)
				require.NoError(t, err)

				err = m.PauseVM(ctx)
				require.NoError(t, err)

				err = m.CreateSnapshot(ctx, memPath, snapPath)
				require.NoError(t, err)

				err = m.StopVMM()
				require.NoError(t, err)
			},

			loadSnapshot: func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string) {
				// Note that many fields are not necessary when loading a snapshot
				cfg := Config{
					SocketPath: socketPath + ".load",
					Drives: []models.Drive{
						{
							DriveID:      String("root"),
							IsRootDevice: Bool(true),
							IsReadOnly:   Bool(true),
							PathOnHost:   String(testRootfs),
						},
					},
				}

				m, err := NewMachine(ctx, cfg, func(m *Machine) {
					// Rewriting m.cmd partially wouldn't work since Cmd has
					// some unexported members
					args := m.cmd.Args[1:]
					m.cmd = exec.Command(getFirecrackerBinaryPath(), args...)
				}, WithLogger(logrus.NewEntry(machineLogger)), WithSnapshot("", snapPath, WithMemoryBackend("File", memPath)))
				require.NoError(t, err)

				err = m.Start(ctx)
				require.NoError(t, err)

				err = m.StopVMM()
				require.NoError(t, err)
			},
		},
		{
			name: "TestLoadSnapshot without create",
			createSnapshot: func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string) {

			},

			loadSnapshot: func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string) {
				cfg := createValidConfig(t, socketPath+".load")
				m, err := NewMachine(ctx, cfg, func(m *Machine) {
					// Rewriting m.cmd partially wouldn't work since Cmd has
					// some unexported members
					args := m.cmd.Args[1:]
					m.cmd = exec.Command(getFirecrackerBinaryPath(), args...)
				}, WithLogger(logrus.NewEntry(machineLogger)), WithSnapshot(memPath, snapPath))
				require.NoError(t, err)

				err = m.Start(ctx)
				require.Error(t, err)
			},
		},
		{
			name: "TestLoadSnapshot and check contents (via ssh)",
			createSnapshot: func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string) {
				cniConfPath := fmt.Sprintf("%s/%s.conflist", cniConfDir, networkName)
				err := writeCNIConfWithHostLocalSubnet(cniConfPath, networkName, subnet)
				require.NoError(t, err)
				defer os.Remove(cniConfPath)

				networkInterface := NetworkInterface{
					CNIConfiguration: &CNIConfiguration{
						NetworkName: networkName,
						IfName:      ifName,
						ConfDir:     cniConfDir,
						BinPath:     cniBinPath,
						VMIfName:    "eth0",
					},
				}
				cfg := createValidConfig(t, fmt.Sprintf("%s.create", socketPath),
					withRootDrive(rootfsPath),
					withNetworkInterface(networkInterface),
				)

				cmd := VMCommandBuilder{}.WithSocketPath(fmt.Sprintf("%s.create", socketPath)).WithBin(getFirecrackerBinaryPath()).Build(ctx)

				m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd))
				require.NoError(t, err)

				err = m.Start(ctx)
				require.NoError(t, err)
				defer m.StopVMM()
				defer func() {
					if err := m.Shutdown(ctx); err != nil {
						t.Logf("error on vm shutdown: %v", err)
					}
				}()

				var client *ssh.Client
				for i := 0; i < maxRetries; i++ {
					client, err = connectToVM(m, sshKeyPath)
					if err != nil {
						time.Sleep(backoffTimeMs * time.Millisecond)
					} else {
						break
					}
				}
				require.NoError(t, err)
				defer client.Close()

				ipToRestore = m.Cfg.NetworkInterfaces.staticIPInterface().StaticConfiguration.IPConfiguration.IPAddr.IP.String()

				session, err := client.NewSession()
				require.NoError(t, err)
				defer session.Close()

				err = session.Start(`sleep 422`) // Should be asynchronous
				require.NoError(t, err)

				err = m.PauseVM(ctx)
				require.NoError(t, err)

				err = m.CreateSnapshot(ctx, memPath, snapPath)
				require.NoError(t, err)
			},

			loadSnapshot: func(ctx context.Context, machineLogger *logrus.Logger, socketPath, memPath, snapPath string) {
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
				require.NoError(t, err)

				cniConfPath := fmt.Sprintf("%s/%s.conflist", cniConfDir, networkName)
				err = writeCNIConfWithHostLocalSubnet(cniConfPath, networkName, subnet)
				require.NoError(t, err)
				defer os.Remove(cniConfPath)

				networkInterface := NetworkInterface{
					CNIConfiguration: &CNIConfiguration{
						NetworkName: networkName,
						IfName:      ifName,
						ConfDir:     cniConfDir,
						BinPath:     cniBinPath,
						Args:        [][2]string{{"IP", ipToRestore + networkMask}},
						VMIfName:    "eth0",
					},
				}

				cfg := Config{
					SocketPath: socketPath + ".load",
					Drives: []models.Drive{
						{
							DriveID:      String("root"),
							IsRootDevice: Bool(true),
							IsReadOnly:   Bool(true),
							PathOnHost:   String(rootfsPath),
						},
					},
					NetworkInterfaces: []NetworkInterface{
						networkInterface,
					},
				}

				cmd := VMCommandBuilder{}.WithSocketPath(fmt.Sprintf("%s.load", socketPath)).WithBin(getFirecrackerBinaryPath()).Build(ctx)

				m, err := NewMachine(ctx, cfg, WithProcessRunner(cmd), WithSnapshot(memPath, snapPath))
				require.NoError(t, err)

				err = m.Start(ctx)
				require.NoError(t, err)
				defer m.StopVMM()

				err = m.ResumeVM(ctx)
				require.NoError(t, err)

				var client *ssh.Client

				for i := 0; i < maxRetries; i++ {
					client, err = connectToVM(m, sshKeyPath)
					if err != nil {
						time.Sleep(backoffTimeMs * time.Millisecond)
					} else {
						break
					}
				}
				require.NoError(t, err)
				defer client.Close()

				session, err := client.NewSession()
				require.NoError(t, err)
				defer session.Close()

				var b bytes.Buffer
				session.Stdout = &b
				err = session.Run(`ps -aux | grep "sleep 422" | wc -l`)
				require.NoError(t, err)
				require.Equal(t, "3\n", b.String())
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()

			// Set snap and mem paths
			socketPath := filepath.Join(dir, fsSafeTestName.Replace(t.Name()))
			snapPath := socketPath + "SnapFile"
			memPath := socketPath + "MemFile"
			defer os.Remove(socketPath)
			defer os.Remove(snapPath)
			defer os.Remove(memPath)

			// Tee logs for validation:
			var logBuffer bytes.Buffer
			machineLogger := logrus.New()
			machineLogger.Out = io.MultiWriter(os.Stderr, &logBuffer)

			c.createSnapshot(ctx, machineLogger, socketPath, snapPath, memPath)
			c.loadSnapshot(ctx, machineLogger, socketPath, snapPath, memPath)
		})
	}

}

func testCreateBalloon(ctx context.Context, t *testing.T, m *Machine) {
	if err := m.CreateBalloon(ctx, testBalloonMemory, testBalloonDeflateOnOom, testStatsPollingIntervals); err != nil {
		t.Errorf("Create balloon device failed from testAttachBalloon: %s", err)
	}
}

func testGetBalloonConfig(ctx context.Context, t *testing.T, m *Machine) {
	expectedBalloonConfig := models.Balloon{
		AmountMib:             &testBalloonMemory,
		DeflateOnOom:          &testBalloonDeflateOnOom,
		StatsPollingIntervals: testStatsPollingIntervals,
	}

	balloonConfig, err := m.GetBalloonConfig(ctx)
	if err != nil {
		t.Errorf("failed to get config: %s", err)
	}
	assert.Equal(t, expectedBalloonConfig, balloonConfig)
}

func testUpdateBalloon(ctx context.Context, t *testing.T, m *Machine) {
	if err := m.UpdateBalloon(ctx, testBalloonNewMemory); err != nil {
		t.Errorf("Updating balloon device failed from testUpdateBalloon: %s", err)
	}
}

func testGetBalloonStats(ctx context.Context, t *testing.T, m *Machine) {
	expectedBalloonStats := models.BalloonStats{
		TargetMib: &testBalloonMemory,
	}

	balloonStat, err := m.GetBalloonStats(ctx)
	if err != nil {
		t.Errorf("failed to get balloon statistics: %s", err)
	}

	assert.Equal(t, expectedBalloonStats.TargetMib, balloonStat.TargetMib)
}

func testUpdateBalloonStats(ctx context.Context, t *testing.T, m *Machine) {
	if err := m.UpdateBalloonStats(ctx, testNewStatsPollingIntervals); err != nil {
		t.Errorf("Updating balloon staistics failed from testUpdateBalloonStats: %s", err)
	}
}
