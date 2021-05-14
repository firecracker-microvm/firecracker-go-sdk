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

package fctesting

import (
	"fmt"
	"os"
	"os/user"
	"testing"

	"golang.org/x/sys/unix"

	log "github.com/sirupsen/logrus"
)

const rootDisableEnvName = "DISABLE_ROOT_TESTS"
const logLevelEnvName = "FC_TEST_LOG_LEVEL"

var rootDisabled bool

func init() {
	if v := os.Getenv(rootDisableEnvName); len(v) != 0 {
		rootDisabled = true
	}
}

func RequiresKVM(t testing.TB) {
	accessErr := unix.Access("/dev/kvm", unix.W_OK)
	if accessErr != nil {
		var name string
		u, err := user.Current()
		if err == nil {
			name = u.Name
		}

		// On GitHub Actions, user.Current() doesn't return an error, but the name is "".
		if name == "" {
			name = fmt.Sprintf("uid=%d", os.Getuid())
		}
		t.Skipf("/dev/kvm is not writable from %s: %s", name, accessErr)
	}
}

// RequiresRoot will ensure that tests that require root access are actually
// root. In addition, this will skip root tests if the DISABLE_ROOT_TESTS is
// set to true
func RequiresRoot(t testing.TB) {
	if rootDisabled {
		t.Skip("skipping test that requires root")
	}

	if e, a := 0, os.Getuid(); e != a {
		t.Fatalf("This test must be run as root. "+
			"To disable tests that require root, "+
			"run the tests with the %s environment variable set.",
			rootDisableEnvName)
	}
}

func newLogger(t testing.TB) *log.Logger {
	str := os.Getenv(logLevelEnvName)
	l := log.New()
	if str == "" {
		return l
	}

	logLevel, err := log.ParseLevel(str)
	if err != nil {
		t.Fatalf("Failed to parse %q as Log Level: %v", str, err)
	}

	l.SetLevel(logLevel)
	return l
}

// NewLogEntry creates log.Entry. The level is specified by "FC_TEST_LOG_LEVEL" environment variable
func NewLogEntry(t testing.TB) *log.Entry {
	return log.NewEntry(newLogger(t))
}
