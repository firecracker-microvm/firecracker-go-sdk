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
	"path/filepath"
	"reflect"
	"testing"
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
				CgroupArgs:     []string{"cpu.shares=10"},
				ChrootStrategy: NewNaiveChrootStrategy("kernel-image-path"),
				ExecFile:       "/path/to/firecracker",
				ChrootBaseDir:  "/tmp",
				JailerBinary:   "/path/to/the/jailer",
				CgroupVersion:  "2",
				ParentCgroup:   "/path/to/parent-cgroup",
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
				"--cgroup",
				"cpu.shares=10",
				"--cgroup-version",
				"2",
				"--parent-cgroup",
				"/path/to/parent-cgroup",
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
				WithCgroupArgs(c.jailerCfg.CgroupArgs...).
				WithCgroupVersion(c.jailerCfg.CgroupVersion).
				WithParentCgroup(c.jailerCfg.ParentCgroup).
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
	var testCases = []struct {
		name             string
		jailerCfg        JailerConfig
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
				ParentCgroup:   "/path/to/parent-cgroup",
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
				"--parent-cgroup",
				"/path/to/parent-cgroup",
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
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			m := &Machine{
				Handlers: Handlers{
					FcInit: defaultFcInitHandlerList,
				},
			}
			cfg := &Config{
				VMID:       "vmid",
				JailerCfg:  &c.jailerCfg,
				NetNS:      c.netns,
				SocketPath: c.socketPath,
			}
			jail(context.Background(), m, cfg)

			if e, a := c.expectedArgs, m.cmd.Args; !reflect.DeepEqual(e, a) {
				t.Errorf("expected args %v, but received %v", e, a)
			}

			if e, a := c.expectedSockPath, cfg.SocketPath; e != a {
				t.Errorf("expected socket path %q, but received %q", e, a)
			}

			foundJailerHandler := false
			for _, handler := range m.Handlers.FcInit.list {
				if handler.Name == LinkFilesToRootFSHandlerName {
					foundJailerHandler = true
					break
				}
			}

			if !foundJailerHandler {
				t.Errorf("did not find link files handler")
			}
		})
	}
}
