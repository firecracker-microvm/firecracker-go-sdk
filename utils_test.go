package firecracker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvValueOrDefaultInt(t *testing.T) {
	defaultVal := 500
	assert.Equal(t, defaultVal, envValueOrDefaultInt("UNEXISTS_ENV", defaultVal))
}
