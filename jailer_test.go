package firecracker

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestJailerBuilder(t *testing.T) {
	cases := []struct {
		name         string
		jailerCfg    JailerConfig
		expectedArgs []string
		jailerBin    string
	}{
		{
			name: "required fields",
			jailerCfg: JailerConfig{
				ID:       "my-test-id",
				UID:      Int(123),
				GID:      Int(100),
				NumaNode: Int(0),
				ExecFile: "/path/to/firecracker",
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
		},
		{
			name:      "optional fields",
			jailerBin: "foo",
			jailerCfg: JailerConfig{
				ID:            "my-test-id",
				UID:           Int(123),
				GID:           Int(100),
				NumaNode:      Int(1),
				ExecFile:      "/path/to/firecracker",
				NetNS:         "/net/namespace",
				ChrootBaseDir: "/tmp",
				SeccompLevel:  SeccompLevelAdvanced,
			},
			expectedArgs: []string{
				"foo",
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
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := NewJailerCommandBuilder().
				WithID(c.jailerCfg.ID).
				WithUID(IntValue(c.jailerCfg.UID)).
				WithGID(IntValue(c.jailerCfg.GID)).
				WithNumaNode(IntValue(c.jailerCfg.NumaNode)).
				WithSeccompLevel(c.jailerCfg.SeccompLevel).
				WithExecFile(c.jailerCfg.ExecFile)

			if len(c.jailerBin) > 0 {
				b = b.WithBin(c.jailerBin)
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
	m := &Machine{
		Handlers: Handlers{
			FcInit: HandlerList{}.Append(CreateMachineHandler),
		},
	}
	cfg := &Config{
		JailerCfg: JailerConfig{
			ID:                "test-id",
			UID:               Int(123),
			GID:               Int(456),
			NumaNode:          Int(0),
			ExecFile:          "/path/to/firecracker",
			DevMapperStrategy: NewNaiveDevMapperStrategy("path", "kernel-image-path"),
		},
	}
	jail(context.Background(), m, cfg)

	expectedArgs := []string{
		defaultJailerBin,
		"--id",
		"test-id",
		"--uid",
		"123",
		"--gid",
		"456",
		"--exec-file",
		"/path/to/firecracker",
		"--node",
		"0",
		"--seccomp-level",
		"0",
	}

	if e, a := expectedArgs, m.cmd.Args; !reflect.DeepEqual(e, a) {
		t.Errorf("expected args %v, but received %v", e, a)
	}

	if e, a := filepath.Join(defaultJailerPath, cfg.JailerCfg.ID, "api.socket"), cfg.SocketPath; e != a {
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
}
