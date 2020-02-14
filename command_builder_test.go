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
	"bytes"
	"context"
	"reflect"
	"testing"
)

func TestVMCommandBuilder(t *testing.T) {
	t.Run("immutability", testVMCommandBuilderImmutable)
	t.Run("chaining", testVMCommandBuilderChaining)
	t.Run("build", testVMCommandBuilderBuild)
}

func testVMCommandBuilderImmutable(t *testing.T) {
	b := VMCommandBuilder{}
	b.WithSocketPath("foo").
		WithArgs([]string{"baz", "qux"}).
		AddArgs("moo", "cow")

	if e, a := []string(nil), b.SocketPath(); !reflect.DeepEqual(e, a) {
		t.Errorf("expected immutable builder, but socket path was set: %q", a)
	}

	if e, a := ([]string)(nil), b.Args(); !reflect.DeepEqual(e, a) {
		t.Errorf("args should have been empty, but received %v", a)
	}
}

func testVMCommandBuilderChaining(t *testing.T) {
	b := VMCommandBuilder{}.
		WithSocketPath("socket-path").
		WithBin("bin")

	if e, a := []string{"--api-sock", "socket-path"}, b.SocketPath(); !reflect.DeepEqual(e, a) {
		t.Errorf("expected %v, but received %v", e, a)
	}

	if e, a := "bin", b.Bin(); e != a {
		t.Errorf("expected %v, but received %v", e, a)
	}
}

func testVMCommandBuilderBuild(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := &bytes.Buffer{}

	ctx := context.Background()
	b := VMCommandBuilder{}.
		WithSocketPath("socket-path").
		WithBin("bin").
		WithStdout(stdout).
		WithStderr(stderr).
		WithStdin(stdin).
		WithArgs([]string{"foo"}).
		AddArgs("--bar", "baz")
	cmd := b.Build(ctx)

	expectedArgs := []string{
		"bin",
		"--api-sock",
		"socket-path",
		"foo",
		"--bar",
		"baz",
	}

	if e, a := stdout, cmd.Stdout; !reflect.DeepEqual(e, a) {
		t.Errorf("expected stdout, %v, but received %v", e, a)
	}

	if e, a := stderr, cmd.Stderr; !reflect.DeepEqual(e, a) {
		t.Errorf("expected stderr, %v, but received %v", e, a)
	}

	if e, a := stdin, cmd.Stdin; !reflect.DeepEqual(e, a) {
		t.Errorf("expected stdin, %v, but received %v", e, a)
	}

	if e, a := expectedArgs, cmd.Args; !reflect.DeepEqual(e, a) {
		t.Errorf("expected %v, but received an invalid set of arguments %v", e, a)
	}
}
