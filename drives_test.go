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
		t.Errorf("expected drives %+v, but received %+v", e, a)
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
	drives := b.WithRootDrive(expectedPath, WithDriveID("foo")).Build()

	if e, a := expectedDrives, drives; !reflect.DeepEqual(e, a) {
		t.Errorf("expected drives %+v, but received %+v", e, a)
	}
}

func TestDrivesBuilderWithCacheType(t *testing.T) {
	expectedPath := "/path/to/rootfs"
	expectedCacheType := models.DriveCacheTypeWriteback
	expectedDrives := []models.Drive{
		{
			DriveID:      String("root_drive"),
			PathOnHost:   &expectedPath,
			IsRootDevice: Bool(true),
			IsReadOnly:   Bool(false),
			CacheType:    String(expectedCacheType),
		},
	}

	b := NewDrivesBuilder(expectedPath)
	drives := b.WithRootDrive(expectedPath, WithDriveID("root_drive"), WithCacheType("Writeback")).Build()

	if e, a := expectedDrives, drives; !reflect.DeepEqual(e, a) {
		t.Errorf("expected drives %+v, but received %+v", e, a)
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
		{
			Path:     "/5",
			ReadOnly: true,
			Opt: func(drive *models.Drive) {
				drive.CacheType = String(models.DriveCacheTypeWriteback)
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
			DriveID:      String("3"),
			PathOnHost:   String("/5"),
			IsRootDevice: Bool(false),
			IsReadOnly:   Bool(true),
			CacheType:    String(models.DriveCacheTypeWriteback),
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
		t.Errorf("expected drives %+v\n, but received %+v", e, a)
	}
}

func TestDrivesBuilderWithIoEngine(t *testing.T) {
	expectedPath := "/path/to/rootfs"
	expectedVal := "Async"
	expectedDrives := []models.Drive{
		{
			DriveID:      String(rootDriveName),
			PathOnHost:   &expectedPath,
			IsRootDevice: Bool(true),
			IsReadOnly:   Bool(false),
			IoEngine:     &expectedVal,
		},
	}

	drives := NewDrivesBuilder(expectedPath).WithRootDrive(expectedPath,
		WithDriveID(string(rootDriveName)), WithIoEngine(expectedVal)).Build()
	if e, a := expectedDrives, drives; !reflect.DeepEqual(e, a) {
		t.Errorf("expected drives %+v, but received %+v", e, a)
	}
}
