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

var (
	expectedAmountMib              = int64(6)
	expectedDeflateOnOom          = true
	expectedStatsPollingIntervals = int64(1)

	expectedBalloon = models.Balloon{
		AmountMib:              &expectedAmountMib,
		DeflateOnOom:          &expectedDeflateOnOom,
		StatsPollingIntervals: expectedStatsPollingIntervals,
	}
)

func TestNewBalloonDevice(t *testing.T) {
	balloon := NewBalloonDevice(expectedAmountMib, expectedDeflateOnOom, WithStatsPollingIntervals(expectedStatsPollingIntervals)).Build()
	if e, a := expectedBalloon, balloon; !reflect.DeepEqual(e, a) {
		t.Errorf("expected balloon %v, but received %v", e, a)
	}
}

func TestUpdateAmountMiB(t *testing.T) {
	BalloonDevice := NewBalloonDevice(int64(1), expectedDeflateOnOom, WithStatsPollingIntervals(expectedStatsPollingIntervals))
	balloon := BalloonDevice.UpdateAmountMib(expectedAmountMib).Build()

	if e, a := expectedBalloon, balloon; !reflect.DeepEqual(e, a) {
		t.Errorf("expected balloon %v, but received %v", e, a)
	}
}

func TestUpdateStatsPollingIntervals(t *testing.T) {
	BalloonDevice := NewBalloonDevice(expectedAmountMib, expectedDeflateOnOom)
	balloon := BalloonDevice.UpdateStatsPollingIntervals(expectedStatsPollingIntervals).Build()

	if e, a := expectedBalloon, balloon; !reflect.DeepEqual(e, a) {
		t.Errorf("expected balloon %v, but received %v", e, a)
	}
}
