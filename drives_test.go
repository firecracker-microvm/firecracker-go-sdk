package firecracker

import (
	"reflect"
	"testing"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

func TestDrivesBuilder(t *testing.T) {
	expectedPath := "/path/to/rootfs"
	expectedDrives := []models.Drive{
		{
			DriveID:      String(rootDriveName),
			PathOnHost:   &expectedPath,
			IsRootDevice: Bool(true),
			IsReadOnly:   Bool(false),
		},
	}

	drives := NewDrivesBuilder(expectedPath).Build()
	if e, a := expectedDrives, drives; !reflect.DeepEqual(e, a) {
		t.Errorf("expected drives %v, but received %v", e, a)
	}
}

func TestDrivesBuilderWithRootDrive(t *testing.T) {
	expectedPath := "/path/to/rootfs"
	expectedDrives := []models.Drive{
		{
			DriveID:      String("foo"),
			PathOnHost:   &expectedPath,
			IsRootDevice: Bool(true),
			IsReadOnly:   Bool(false),
		},
	}

	b := NewDrivesBuilder(expectedPath)
	drives := b.WithRootDrive(expectedPath, func(drive *models.Drive) {
		drive.DriveID = String("foo")
	}).Build()

	if e, a := expectedDrives, drives; !reflect.DeepEqual(e, a) {
		t.Errorf("expected drives %v, but received %v", e, a)
	}
}

func TestDrivesBuilderAddDrive(t *testing.T) {
	rootPath := "/root/path"
	drivesToAdd := []struct {
		Path     string
		ReadOnly bool
		Opt      func(drive *models.Drive)
	}{
		{
			Path:     "/2",
			ReadOnly: true,
		},
		{
			Path:     "/3",
			ReadOnly: false,
		},
		{
			Path:     "/4",
			ReadOnly: false,
			Opt: func(drive *models.Drive) {
				drive.Partuuid = "uuid"
			},
		},
	}
	expectedDrives := []models.Drive{
		{
			DriveID:      String("0"),
			PathOnHost:   String("/2"),
			IsRootDevice: Bool(false),
			IsReadOnly:   Bool(true),
		},
		{
			DriveID:      String("1"),
			PathOnHost:   String("/3"),
			IsRootDevice: Bool(false),
			IsReadOnly:   Bool(false),
		},
		{
			DriveID:      String("2"),
			PathOnHost:   String("/4"),
			IsRootDevice: Bool(false),
			IsReadOnly:   Bool(false),
			Partuuid:     "uuid",
		},
		{
			DriveID:      String(rootDriveName),
			PathOnHost:   &rootPath,
			IsRootDevice: Bool(true),
			IsReadOnly:   Bool(false),
		},
	}

	b := NewDrivesBuilder(rootPath)
	for _, drive := range drivesToAdd {
		if drive.Opt != nil {
			b = b.AddDrive(drive.Path, drive.ReadOnly, drive.Opt)
		} else {
			b = b.AddDrive(drive.Path, drive.ReadOnly)
		}
	}

	drives := b.Build()
	if e, a := expectedDrives, drives; !reflect.DeepEqual(e, a) {
		t.Errorf("expected drives %v, but received %v", e, a)
	}
}
