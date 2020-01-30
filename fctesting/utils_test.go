package fctesting

import (
	"os"
	"testing"
)

func TestLoggingPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Error(r)
		}
	}()

	os.Setenv("FC_TEST_LOG_LEVEL", "debug")
	l := NewLogEntry(t)
	l.Debug("TestLoggingPanic")
}
