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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKernelArgsSerder(t *testing.T) {
	fooVal := "bar"
	booVal := "far"
	dooVal := "a=silly=val"
	initVal := "/bin/sh -- -c \"echo hello\""
	emptyVal := ""

	argsString := fmt.Sprintf("foo=%s blah doo=%s huh=%s bleh duh=%s boo=%s init=%s",
		fooVal,
		dooVal,
		emptyVal,
		emptyVal,
		booVal,
		initVal,
	)

	expectedParsedArgs := kernelArgs(map[string]*string{
		"foo":  &fooVal,
		"doo":  &dooVal,
		"init": &initVal,
		"blah": nil,
		"huh":  &emptyVal,
		"bleh": nil,
		"duh":  &emptyVal,
		"boo":  &booVal,
	})

	actualParsedArgs := parseKernelArgs(argsString)
	fmt.Printf("%v\n", actualParsedArgs)
	fmt.Printf("%v\n", expectedParsedArgs)
	require.Equal(t, expectedParsedArgs, actualParsedArgs, "kernel args parsed to unexpected values")

	reparsedArgs := parseKernelArgs(actualParsedArgs.String())
	require.Equal(t, expectedParsedArgs, reparsedArgs, "serializing and deserializing kernel args did not result in same value")
}
