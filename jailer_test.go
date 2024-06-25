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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

func TestJailerBuilder(t *testing.T) {
	var testCases = []struct {
		name             string
		jailerCfg        JailerConfig
		expectedArgs     []string
		netns            string
		expectedSockPath string
	}{
		{
			name: "required fields",
			jailerCfg: JailerConfig{
				ID:             "my-test-id",
				UID:            Int(123),
				GID:            Int(100),
				NumaNode:       Int(0),
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
			},
			expectedArgs: []string{
				defaultJailerBin,
				"--id",
				"my-test-id",
				"--uid",
				"123",
				"--gid",
				"100",
				"--exec-file",
				"/path/to/firecracker",
				"--cgroup",
				"cpuset.mems=0",
				"--cgroup",
				fmt.Sprintf("cpuset.cpus=%s", getNumaCpuset(0)),
			},
			expectedSockPath: filepath.Join(
				defaultJailerPath,
				"firecracker",
				"my-test-id",
				rootfsFolderName,
				"run",
				"firecracker.socket"),
		},
		{
			name: "other jailer binary name",
			jailerCfg: JailerConfig{
				ID:             "my-test-id",
				UID:            Int(123),
				GID:            Int(100),
				NumaNode:       Int(0),
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
				JailerBinary:   "imprisoner",
			},
			expectedArgs: []string{
				"imprisoner",
				"--id",
				"my-test-id",
				"--uid",
				"123",
				"--gid",
				"100",
				"--exec-file",
				"/path/to/firecracker",
				"--cgroup",
				"cpuset.mems=0",
				"--cgroup",
				fmt.Sprintf("cpuset.cpus=%s", getNumaCpuset(0)),
			},
			expectedSockPath: filepath.Join(
				defaultJailerPath,
				"firecracker",
				"my-test-id",
				rootfsFolderName,
				"run",
				"firecracker.socket"),
		},
		{
			name:  "optional fields",
			netns: "/path/to/netns",
			jailerCfg: JailerConfig{
				ID:             "my-test-id",
				UID:            Int(123),
				GID:            Int(100),
				NumaNode:       Int(0),
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
				ChrootBaseDir:  "/tmp",
				JailerBinary:   "/path/to/the/jailer",
				CgroupVersion:  "2",
			},
			expectedArgs: []string{
				"/path/to/the/jailer",
				"--id",
				"my-test-id",
				"--uid",
				"123",
				"--gid",
				"100",
				"--exec-file",
				"/path/to/firecracker",
				"--cgroup",
				"cpuset.mems=0",
				"--cgroup",
				fmt.Sprintf("cpuset.cpus=%s", getNumaCpuset(0)),
				"--cgroup-version",
				"2",
				"--chroot-base-dir",
				"/tmp",
				"--netns",
				"/path/to/netns",
			},
			expectedSockPath: filepath.Join(
				"/tmp",
				"firecracker",
				"my-test-id",
				rootfsFolderName,
				"run",
				"firecracker.socket"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			b := NewJailerCommandBuilder().
				WithID(c.jailerCfg.ID).
				WithUID(IntValue(c.jailerCfg.UID)).
				WithGID(IntValue(c.jailerCfg.GID)).
				WithNumaNode(IntValue(c.jailerCfg.NumaNode)).
				WithCgroupVersion(c.jailerCfg.CgroupVersion).
				WithExecFile(c.jailerCfg.ExecFile)

			if len(c.jailerCfg.JailerBinary) > 0 {
				b = b.WithBin(c.jailerCfg.JailerBinary)
			}

			if len(c.jailerCfg.ChrootBaseDir) > 0 {
				b = b.WithChrootBaseDir(c.jailerCfg.ChrootBaseDir)
			}

			if c.netns != "" {
				b = b.WithNetNS(c.netns)
			}

			if c.jailerCfg.Daemonize {
				b = b.WithDaemonize(c.jailerCfg.Daemonize)
			}

			cmd := b.Build(context.Background())
			if e, a := c.expectedArgs, cmd.Args; !reflect.DeepEqual(e, a) {
				t.Errorf("expected args %v, but received %v", e, a)
			}
		})
	}
}

func TestJail(t *testing.T) {
	testTempDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("failed to create temp dir for test: %s", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(testTempDir); err != nil {
			t.Errorf("failed to clean up test temp dir: %s", err)
		}
	})

	var testCases = []struct {
		name             string
		jailerCfg        JailerConfig
		drives           []models.Drive
		testLinkFiles    bool
		expectedArgs     []string
		netns            string
		socketPath       string
		expectedSockPath string
	}{
		{
			name: "required fields",
			jailerCfg: JailerConfig{
				ID:             "my-test-id",
				UID:            Int(123),
				GID:            Int(100),
				NumaNode:       Int(0),
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
			},
			expectedArgs: []string{
				defaultJailerBin,
				"--id",
				"my-test-id",
				"--uid",
				"123",
				"--gid",
				"100",
				"--exec-file",
				"/path/to/firecracker",
				"--cgroup",
				"cpuset.mems=0",
				"--cgroup",
				fmt.Sprintf("cpuset.cpus=%s", getNumaCpuset(0)),
				"--",
				"--no-seccomp",
				"--api-sock",
				"/run/firecracker.socket",
			},
			expectedSockPath: filepath.Join(
				defaultJailerPath,
				"firecracker",
				"my-test-id",
				rootfsFolderName,
				"run",
				"firecracker.socket"),
		},
		{
			name: "other jailer binary name",
			jailerCfg: JailerConfig{
				ID:             "my-test-id",
				UID:            Int(123),
				GID:            Int(100),
				NumaNode:       Int(0),
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
				JailerBinary:   "imprisoner",
			},
			expectedArgs: []string{
				"imprisoner",
				"--id",
				"my-test-id",
				"--uid",
				"123",
				"--gid",
				"100",
				"--exec-file",
				"/path/to/firecracker",
				"--cgroup",
				"cpuset.mems=0",
				"--cgroup",
				fmt.Sprintf("cpuset.cpus=%s", getNumaCpuset(0)),
				"--",
				"--no-seccomp",
				"--api-sock",
				"/run/firecracker.socket",
			},
			expectedSockPath: filepath.Join(
				defaultJailerPath,
				"firecracker",
				"my-test-id",
				rootfsFolderName,
				"run",
				"firecracker.socket"),
		},
		{
			name:  "optional fields",
			netns: "/path/to/netns",
			jailerCfg: JailerConfig{
				ID:             "my-test-id",
				UID:            Int(123),
				GID:            Int(100),
				NumaNode:       Int(0),
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
				ChrootBaseDir:  "/tmp",
				JailerBinary:   "/path/to/the/jailer",
				CgroupVersion:  "2",
			},
			expectedArgs: []string{
				"/path/to/the/jailer",
				"--id",
				"my-test-id",
				"--uid",
				"123",
				"--gid",
				"100",
				"--exec-file",
				"/path/to/firecracker",
				"--cgroup",
				"cpuset.mems=0",
				"--cgroup",
				fmt.Sprintf("cpuset.cpus=%s", getNumaCpuset(0)),
				"--cgroup-version",
				"2",
				"--chroot-base-dir",
				"/tmp",
				"--netns",
				"/path/to/netns",
				"--",
				"--no-seccomp",
				"--api-sock",
				"/run/firecracker.socket",
			},
			expectedSockPath: filepath.Join(
				"/tmp",
				"firecracker",
				"my-test-id",
				rootfsFolderName,
				"run",
				"firecracker.socket"),
		},
		{
			name:       "custom socket path",
			socketPath: "api.sock",
			jailerCfg: JailerConfig{
				ID:             "my-test-id",
				UID:            Int(123),
				GID:            Int(100),
				NumaNode:       Int(0),
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
			},
			expectedArgs: []string{
				defaultJailerBin,
				"--id",
				"my-test-id",
				"--uid",
				"123",
				"--gid",
				"100",
				"--exec-file",
				"/path/to/firecracker",
				"--cgroup",
				"cpuset.mems=0",
				"--cgroup",
				fmt.Sprintf("cpuset.cpus=%s", getNumaCpuset(0)),
				"--",
				"--no-seccomp",
				"--api-sock",
				"api.sock",
			},
			expectedSockPath: filepath.Join(
				defaultJailerPath,
				"firecracker",
				"my-test-id",
				rootfsFolderName,
				"api.sock"),
		},
		{
			name: "files already in jailer root",
			jailerCfg: JailerConfig{
				ID:             "my-test-id",
				UID:            Int(123),
				GID:            Int(100),
				NumaNode:       Int(0),
				ChrootBaseDir:  testTempDir,
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
			},
			drives: []models.Drive{
				{
					DriveID:      String("test-drive-id"),
					IsReadOnly:   Bool(true),
					IsRootDevice: Bool(true),
					// Test a host path that is already inside the rootfs
					PathOnHost: String(filepath.Join(
						testTempDir,
						"firecracker",
						"my-test-id",
						rootfsFolderName,
						"image.ext4")),
				},
			},
			expectedArgs: []string{
				defaultJailerBin,
				"--id",
				"my-test-id",
				"--uid",
				"123",
				"--gid",
				"100",
				"--exec-file",
				"/path/to/firecracker",
				"--cgroup",
				"cpuset.mems=0",
				"--cgroup",
				fmt.Sprintf("cpuset.cpus=%s", getNumaCpuset(0)),
				"--chroot-base-dir",
				testTempDir,
				"--",
				"--no-seccomp",
				"--api-sock",
				"/run/firecracker.socket",
			},
			expectedSockPath: filepath.Join(
				testTempDir,
				"firecracker",
				"my-test-id",
				rootfsFolderName,
				"run",
				"firecracker.socket"),
			testLinkFiles: true,
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			// Clear the temp dir between tests.
			if err := os.RemoveAll(testTempDir); err != nil {
				t.Fatalf("failed to clear temp dir: %s", err)
			}
			if err := os.MkdirAll(testTempDir, 0755); err != nil {
				t.Fatalf("failed to create temp dir: %s", err)
			}

			ctx := context.Background()
			m := &Machine{
				Handlers: Handlers{
					FcInit: defaultFcInitHandlerList,
				},
			}
			m.Cfg = Config{
				VMID:            "vmid",
				JailerCfg:       &c.jailerCfg,
				NetNS:           c.netns,
				SocketPath:      c.socketPath,
				Drives:          append([]models.Drive{}, c.drives...), // copy
				KernelImagePath: createEmptyTempFile(t, testTempDir, "kernel-*"),
			}
			jail(ctx, m, &m.Cfg)

			if e, a := c.expectedArgs, m.cmd.Args; !reflect.DeepEqual(e, a) {
				t.Errorf("expected args %v, but received %v", e, a)
			}

			if e, a := c.expectedSockPath, m.Cfg.SocketPath; e != a {
				t.Errorf("expected socket path %q, but received %q", e, a)
			}

			var jailerHandler *Handler
			for _, handler := range m.Handlers.FcInit.list {
				if handler.Name == LinkFilesToRootFSHandlerName {
					jailerHandler = &handler
					break
				}
			}

			if jailerHandler == nil {
				t.Errorf("did not find link files handler")
			}

			if !c.testLinkFiles {
				return
			}

			rootfsPath := filepath.Join(
				testTempDir, "firecracker", "my-test-id", rootfsFolderName,
			)
			if err := os.MkdirAll(rootfsPath, 0755); err != nil {
				t.Fatalf("failed to create rootfs dir: %s", err)
			}
			if err := jailerHandler.Fn(ctx, m); err != nil {
				t.Errorf("failed to run handler: %s", err)
			}

			// Drive paths should be updated to be rootfs-relative.
			for i, d := range m.Cfg.Drives {
				newPath := filepath.Join(rootfsPath, *d.PathOnHost)
				if e, a := *c.drives[i].PathOnHost, newPath; e != a {
					t.Errorf("expected drive host path %q, but received %q", e, a)
				}
			}
		})
	}
}

func createEmptyTempFile(t *testing.T, testTempDir, pattern string) string {
	f, err := os.CreateTemp(testTempDir, pattern)
	if err != nil {
		t.Fatalf("Failed to create temp kernel image: %s", err)
	}
	defer f.Close()
	return f.Name()
}
