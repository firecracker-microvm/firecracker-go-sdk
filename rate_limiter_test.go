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

package firecracker_test

import (
	"testing"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiter(t *testing.T) {
	bucket := firecracker.TokenBucketBuilder{}.
		WithRefillDuration(1 * time.Hour).
		WithBucketSize(100).
		WithInitialSize(100).
		Build()

	expectedBucket := models.TokenBucket{
		OneTimeBurst: firecracker.Int64(100),
		RefillTime:   firecracker.Int64(3600000),
		Size:         firecracker.Int64(100),
	}

	assert.Equal(t, expectedBucket, bucket)
}

func TestRateLimiter_RefillTime(t *testing.T) {
	cases := []struct {
		Name                 string
		Dur                  time.Duration
		ExpectedMilliseconds int64
	}{
		{
			Name:                 "one hour",
			Dur:                  1 * time.Hour,
			ExpectedMilliseconds: 3600000,
		},
		{
			Name:                 "zero",
			ExpectedMilliseconds: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			bucket := firecracker.TokenBucketBuilder{}.
				WithRefillDuration(c.Dur).
				Build()

			assert.Equal(t, &c.ExpectedMilliseconds, bucket.RefillTime)
		})
	}
}
