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

import "github.com/firecracker-microvm/firecracker-go-sdk/client/models"

type SnapshotConfig struct {
	MemFilePath         string
	MemBackend          *models.MemoryBackend
	SnapshotPath        string
	EnableDiffSnapshots bool
	ResumeVM            bool
}

// GetMemBackendPath returns the effective memory backend path. If MemBackend
// is not set, then MemFilePath from SnapshotConfig will be returned.
func (cfg *SnapshotConfig) GetMemBackendPath() string {
	if cfg.MemBackend != nil && cfg.MemBackend.BackendPath != nil {
		return *cfg.MemBackend.BackendPath
	}
	return cfg.MemFilePath
}
