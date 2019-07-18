package firecracker

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

var testCases = []struct {
	name             string
	jailerCfg        JailerConfig
	expectedArgs     []string
	expectedSockPath string
}{
	{
		name: "required fields",
		jailerCfg: JailerConfig{
			ID:             "my-test-id",
			UID:            Int(123),
			GID:            Int(100),
			NumaNode:       Int(0),
			ChrootStrategy: NewNaiveChrootStrategy("path", "kernel-image-path"),
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
			"--node",
			"0",
			"--seccomp-level",
			"0",
		},
		expectedSockPath: filepath.Join(defaultJailerPath, "my-test-id", rootfsFolderName, "api.socket"),
	},
	{
		name: "other jailer binary name",
		jailerCfg: JailerConfig{
			ID:             "my-test-id",
			UID:            Int(123),
			GID:            Int(100),
			NumaNode:       Int(0),
			ChrootStrategy: NewNaiveChrootStrategy("path", "kernel-image-path"),
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
			"--node",
			"0",
			"--seccomp-level",
			"0",
		},
		expectedSockPath: filepath.Join(defaultJailerPath, "my-test-id", rootfsFolderName, "api.socket"),
	},
	{
		name: "optional fields",
		jailerCfg: JailerConfig{
			ID:             "my-test-id",
			UID:            Int(123),
			GID:            Int(100),
			NumaNode:       Int(1),
			ChrootStrategy: NewNaiveChrootStrategy("path", "kernel-image-path"),
			ExecFile:       "/path/to/firecracker",
			NetNS:          "/net/namespace",
			ChrootBaseDir:  "/tmp",
			SeccompLevel:   SeccompLevelAdvanced,
			JailerBinary:   "/path/to/the/jailer",
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
			"--node",
			"1",
			"--chroot-base-dir",
			"/tmp",
			"--netns",
			"/net/namespace",
			"--seccomp-level",
			"2",
		},
		expectedSockPath: filepath.Join("/tmp", "firecracker", "my-test-id", rootfsFolderName, "api.socket"),
	},
}

func TestJailerBuilder(t *testing.T) {
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			b := NewJailerCommandBuilder().
				WithID(c.jailerCfg.ID).
				WithUID(IntValue(c.jailerCfg.UID)).
				WithGID(IntValue(c.jailerCfg.GID)).
				WithNumaNode(IntValue(c.jailerCfg.NumaNode)).
				WithSeccompLevel(c.jailerCfg.SeccompLevel).
				WithExecFile(c.jailerCfg.ExecFile)

			if len(c.jailerCfg.JailerBinary) > 0 {
				b = b.WithBin(c.jailerCfg.JailerBinary)
			}

			if len(c.jailerCfg.ChrootBaseDir) > 0 {
				b = b.WithChrootBaseDir(c.jailerCfg.ChrootBaseDir)
			}

			if len(c.jailerCfg.NetNS) > 0 {
				b = b.WithNetNS(c.jailerCfg.NetNS)
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
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			m := &Machine{
				Handlers: Handlers{
					FcInit: defaultFcInitHandlerList,
				},
			}
			cfg := &Config{
				JailerCfg: c.jailerCfg,
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
