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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

const (
	defaultJailerPath = "/srv/jailer/firecracker"
)

// SeccompLevelValue represents a secure computing level type.
type SeccompLevelValue int

// secure computing levels
const (
	// SeccompLevelDisable is the default value.
	SeccompLevelDisable SeccompLevelValue = iota
	// SeccompLevelBasic prohibits syscalls not whitelisted by Firecracker.
	SeccompLevelBasic
	// SeccompLevelAdvanced adds further checks on some of the parameters of the
	// allowed syscalls.
	SeccompLevelAdvanced
)

// JailerConfig is jailer specific configuration needed to execute the jailer.
type JailerConfig struct {
	GID           *int
	UID           *int
	ID            string
	NumaNode      *int
	ExecFile      string
	ChrootBaseDir string
	NetNS         string
	Daemonize     bool
	SeccompLevel  SeccompLevelValue
}

// JailerCommandBuilder will build a jailer command. This can be used to
// specify that a jailed firecracker executable wants to be run on the Machine.
type JailerCommandBuilder struct {
	id       string
	uid      int
	gid      int
	execFile string
	node     int

	// optional params
	chrootBaseDir string
	netNS         string
	daemonize     bool
	seccompLevel  SeccompLevelValue

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// Args returns the specified set of args to be used
// in command construction.
func (b JailerCommandBuilder) Args() []string {
	args := []string{}
	args = append(args, b.ID()...)
	args = append(args, b.UID()...)
	args = append(args, b.GID()...)
	args = append(args, b.ExecFile()...)
	args = append(args, b.NumaNode()...)

	if len(b.chrootBaseDir) > 0 {
		args = append(args, b.ChrootBaseDir()...)
	}

	if len(b.netNS) > 0 {
		args = append(args, b.NetNS()...)
	}

	args = append(args, b.SeccompLevel()...)

	if b.daemonize {
		args = append(args, "--daemonize")
	}

	return args
}

// ID will return the command arguments regarding the id.
func (b JailerCommandBuilder) ID() []string {
	return []string{
		"--id",
		b.id,
	}
}

// WithID will set the specified id to the builder.
func (b JailerCommandBuilder) WithID(id string) JailerCommandBuilder {
	b.id = id
	return b
}

// UID will return the command arguments regarding the uid.
func (b JailerCommandBuilder) UID() []string {
	return []string{
		"--uid",
		strconv.Itoa(b.uid),
	}
}

// WithUID will set the specified uid to the builder.
func (b JailerCommandBuilder) WithUID(uid int) JailerCommandBuilder {
	b.uid = uid
	return b
}

// GID will return the command arguments regarding the gid.
func (b JailerCommandBuilder) GID() []string {
	return []string{
		"--gid",
		strconv.Itoa(b.gid),
	}
}

// WithGID will set the specified gid to the builder.
func (b JailerCommandBuilder) WithGID(gid int) JailerCommandBuilder {
	b.gid = gid
	return b
}

// ExecFile will return the command arguments regarding the exec file.
func (b JailerCommandBuilder) ExecFile() []string {
	return []string{
		"--exec-file",
		b.execFile,
	}
}

// WithExecFile will set the specified path to the builder. This represents a
// firecracker binary used when calling the jailer.
func (b JailerCommandBuilder) WithExecFile(path string) JailerCommandBuilder {
	b.execFile = path
	return b
}

// NumaNode will return the command arguments regarding the numa node.
func (b JailerCommandBuilder) NumaNode() []string {
	return []string{
		"--node",
		strconv.Itoa(b.node),
	}
}

// WithNumaNode uses the specfied node for the jailer. This represents the numa
// node that the process will get assigned to.
func (b JailerCommandBuilder) WithNumaNode(node int) JailerCommandBuilder {
	b.node = node
	return b
}

// ChrootBaseDir will return the command arguments regarding the chroot base
// directory.
func (b JailerCommandBuilder) ChrootBaseDir() []string {
	return []string{
		"--chroot-base-dir",
		b.chrootBaseDir,
	}
}

// WithChrootBaseDir will set the given path as the chroot base directory. This
// specifies where chroot jails are built and defaults to /srv/jailer.
func (b JailerCommandBuilder) WithChrootBaseDir(path string) JailerCommandBuilder {
	b.chrootBaseDir = path
	return b
}

// NetNS will return the command arguments regarding the net namespace.
func (b JailerCommandBuilder) NetNS() []string {
	return []string{
		"--netns",
		b.netNS,
	}
}

// WithNetNS will set the given path to the net namespace of the builder. This
// represents the path to a network namespace handle and will be used to join
// the associated network namepsace.
func (b JailerCommandBuilder) WithNetNS(path string) JailerCommandBuilder {
	b.netNS = path
	return b
}

// WithDaemonize will specify whether to set stdio to /dev/null
func (b JailerCommandBuilder) WithDaemonize(daemonize bool) JailerCommandBuilder {
	b.daemonize = daemonize
	return b
}

// SeccompLevel will return the command arguments regarding secure computing
// level.
func (b JailerCommandBuilder) SeccompLevel() []string {
	return []string{
		"--seccomp-level",
		strconv.Itoa(int(b.seccompLevel)),
	}
}

// WithSeccompLevel will set the provided level to the builder. This represents
// the seccomp filters that should be installed and how restrictive they should
// be.
func (b JailerCommandBuilder) WithSeccompLevel(level SeccompLevelValue) JailerCommandBuilder {
	b.seccompLevel = level
	return b
}

// Stdout will return the stdout that will be used when creating the
// firecracker exec.Command
func (b JailerCommandBuilder) Stdout() io.Writer {
	return b.stdout
}

// WithStdout specifies which io.Writer to use in place of the os.Stdout in the
// firecracker exec.Command.
func (b JailerCommandBuilder) WithStdout(stdout io.Writer) JailerCommandBuilder {
	b.stdout = stdout
	return b
}

// Stderr will return the stderr that will be used when creating the
// firecracker exec.Command
func (b JailerCommandBuilder) Stderr() io.Writer {
	return b.stderr
}

// WithStderr specifies which io.Writer to use in place of the os.Stderr in the
// firecracker exec.Command.
func (b JailerCommandBuilder) WithStderr(stderr io.Writer) JailerCommandBuilder {
	b.stderr = stderr
	return b
}

// Stdin will return the stdin that will be used when creating the firecracker
// exec.Command
func (b JailerCommandBuilder) Stdin() io.Reader {
	return b.stdin
}

// WithStdin specifies which io.Reader to use in place of the os.Stdin in the
// firecracker exec.Command.
func (b JailerCommandBuilder) WithStdin(stdin io.Reader) JailerCommandBuilder {
	b.stdin = stdin
	return b
}

// Build will build a jailer command.
func (b JailerCommandBuilder) Build(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(
		ctx,
		"jailer",
		b.Args()...,
	)

	if stdin := b.Stdin(); stdin != nil {
		cmd.Stdin = stdin
	}

	if stdout := b.Stdout(); stdout != nil {
		cmd.Stdout = stdout
	}

	if stderr := b.Stderr(); stderr != nil {
		cmd.Stderr = stderr
	}

	return cmd
}

// Jail will set up proper handlers and remove configuuration validation due to
// stating of files
func jail(ctx context.Context, m *Machine, cfg *Config) {
	rootfs := ""
	if len(cfg.JailerCfg.ChrootBaseDir) > 0 {
		rootfs = filepath.Join(cfg.JailerCfg.ChrootBaseDir, "firecracker", cfg.JailerCfg.ID)
	} else {
		rootfs = filepath.Join(defaultJailerPath, cfg.JailerCfg.ID)
	}

	cfg.SocketPath = filepath.Join(rootfs, "api.socket")
	m.cmd = JailerCommandBuilder{}.
		WithID(cfg.JailerCfg.ID).
		WithUID(IntValue(cfg.JailerCfg.UID)).
		WithGID(IntValue(cfg.JailerCfg.GID)).
		WithNumaNode(IntValue(cfg.JailerCfg.NumaNode)).
		WithExecFile(cfg.JailerCfg.ExecFile).
		WithChrootBaseDir(cfg.JailerCfg.ChrootBaseDir).
		WithDaemonize(cfg.JailerCfg.Daemonize).
		WithSeccompLevel(cfg.JailerCfg.SeccompLevel).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		Build(ctx)

	// ValidateCfgHandlerName handler needs to be removed due to checking for
	// non-existant drive paths and socket path. Since the jailer uses relative
	// links according to its rootfs then the user would also need those files
	// relative to running this.
	m.Handlers.Validation = m.Handlers.Validation.Remove(ValidateCfgHandlerName)
	m.Handlers.FcInit = m.Handlers.FcInit.AppendAfter(CreateMachineHandlerName, Handler{
		Name: "fcinit.CopyFilesToRootFS",
		Fn: func(ctx context.Context, m *Machine) error {

			// copy kernel image to root fs
			kernelImageFileName := filepath.Base(m.cfg.KernelImagePath)
			if err := linkFileToRootFS(
				m.cfg.JailerCfg,
				filepath.Join(rootfs, "root", kernelImageFileName),
				m.cfg.KernelImagePath,
			); err != nil {
				return err
			}

			var rootDrive models.Drive
			rootDriveIndex := 0
			for i, drive := range m.cfg.Drives {
				if BoolValue(drive.IsRootDevice) {
					rootDrive = drive
					rootDriveIndex = i
					break
				}
			}

			rootHostPath := StringValue(rootDrive.PathOnHost)
			// copy root drive to root fs
			rootdriveFileName := filepath.Base(rootHostPath)
			if err := linkFileToRootFS(
				m.cfg.JailerCfg,
				filepath.Join(rootfs, "root", rootdriveFileName),
				rootHostPath,
			); err != nil {
				return err
			}

			m.cfg.Drives[rootDriveIndex].PathOnHost = &rootdriveFileName
			m.cfg.KernelImagePath = kernelImageFileName
			return nil
		},
	})
}

func linkFileToRootFS(cfg JailerConfig, dst, src string) error {
	if err := os.Link(src, dst); err != nil {
		return err
	}

	return os.Chown(dst, IntValue(cfg.UID), IntValue(cfg.GID))
}
