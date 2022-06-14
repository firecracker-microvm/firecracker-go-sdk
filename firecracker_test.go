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
	"path/filepath"
	"testing"
	"time"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/firecracker-microvm/firecracker-go-sdk/fctesting"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()
	socketpath, cleanup := makeSocketPath(t)
	defer cleanup()

	cmd := VMCommandBuilder{}.
		WithBin(getFirecrackerBinaryPath()).
		WithSocketPath(socketpath).
		Build(ctx)

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start firecracker vmm: %v", err)
	}

	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Errorf("failed to kill process: %v", err)
		}
	}()

	drive := &models.Drive{
		DriveID:      String("test"),
		IsReadOnly:   Bool(false),
		IsRootDevice: Bool(false),
		PathOnHost:   String(filepath.Join(testDataPath, "drive-2.img")),
	}

	client := NewClient(socketpath, fctesting.NewLogEntry(t), true)
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer deadlineCancel()
	if err := waitForAliveVMM(deadlineCtx, client); err != nil {
		t.Fatal(err)
	}

	if _, err := client.PutGuestDriveByID(ctx, "test", drive); err != nil {
		t.Errorf("unexpected error on PutGuestDriveByID, %v", err)
	}
}

func TestGetFirecrackerVersion(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()
	socketpath, cleanup := makeSocketPath(t)
	defer cleanup()

	cmd := VMCommandBuilder{}.
		WithBin(getFirecrackerBinaryPath()).
		WithSocketPath(socketpath).
		Build(ctx)

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start firecracker vmm: %v", err)
	}

	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Errorf("failed to kill process: %v", err)
		}
	}()

	client := NewClient(socketpath, fctesting.NewLogEntry(t), true)
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer deadlineCancel()
	if err := waitForAliveVMM(deadlineCtx, client); err != nil {
		t.Fatal(err)
	}

	_, err := client.GetFirecrackerVersion(ctx)
	require.NoError(t, err, "failed to get firecracker version")
}
