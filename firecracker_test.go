package firecracker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/firecracker-microvm/firecracker-go-sdk/fctesting"
)

func TestClient(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()
	socketpath := filepath.Join(testDataPath, "test.socket")

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
		os.Remove(socketpath)
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

	if _, err := client.DescribeInstance(ctx); err != nil {
		t.Errorf("unexpected error on DescribeInstance, %v", err)
	}
}
