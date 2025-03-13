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
	"os/exec"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/sirupsen/logrus"
)

// Opt represents a functional option to help modify functionality of a Machine.
type Opt func(*Machine)

// WithClient will use the client in place rather than the client constructed
// during bootstrapping of the machine. This option is useful for mocking out
// tests.
func WithClient(client *Client) Opt {
	return func(machine *Machine) {
		machine.client = client
	}
}

// WithLogger will allow for the Machine to use the provided logger.
func WithLogger(logger *logrus.Entry) Opt {
	return func(machine *Machine) {
		machine.logger = logger
	}
}

// WithProcessRunner will allow for a specific command to be run instead of the
// default firecracker command.
// For example, this could be used to instead call the jailer instead of
// firecracker directly.
func WithProcessRunner(cmd *exec.Cmd) Opt {
	return func(machine *Machine) {
		machine.cmd = cmd
	}
}

// WithSnapshotOpt allows configuration of the snapshot config
// to be passed to LoadSnapshot
type WithSnapshotOpt func(*SnapshotConfig)

// WithSnapshot will allow for the machine to start using a given snapshot.
//
// If using the UFFD memory backend, the memFilePath may be empty (it is
// ignored), and instead the UFFD socket should be specified using
// MemoryBackendType, as in the following example:
//
//	WithSnapshot(
//	  "", snapshotPath,
//	  WithMemoryBackend(models.MemoryBackendBackendTypeUffd, "uffd.sock"))
func WithSnapshot(memFilePath, snapshotPath string, opts ...WithSnapshotOpt) Opt {
	return func(m *Machine) {
		m.Cfg.Snapshot.MemFilePath = memFilePath
		m.Cfg.Snapshot.SnapshotPath = snapshotPath

		for _, opt := range opts {
			opt(&m.Cfg.Snapshot)
		}

		m.Handlers.Validation = m.Handlers.Validation.Remove(ValidateCfgHandlerName).Append(LoadSnapshotConfigValidationHandler)
		m.Handlers.FcInit = modifyHandlersForLoadSnapshot(m.Handlers.FcInit)
	}
}

func modifyHandlersForLoadSnapshot(l HandlerList) HandlerList {
	for _, h := range loadSnapshotRemoveHandlerList {
		l = l.Remove(h.Name)
	}
	l = l.Append(LoadSnapshotHandler)
	return l
}

// WithMemoryBackend sets the memory backend to the given type, using the given
// backing file path (a regular file for "File" type, or a UFFD socket path for
// "Uffd" type).
//
// Note that if MemFilePath is already configured for the snapshot config, it
// will be ignored, and the backendPath specified here will be used instead.
func WithMemoryBackend(backendType, backendPath string) WithSnapshotOpt {
	return func(cfg *SnapshotConfig) {
		cfg.MemBackend = &models.MemoryBackend{
			BackendType: String(backendType),
			BackendPath: String(backendPath),
		}
	}
}
