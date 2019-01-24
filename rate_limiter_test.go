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
