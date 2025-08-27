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
	emptyVal := ""

	argsString := fmt.Sprintf("foo=%s blah doo=%s huh=%s bleh duh=%s boo=%s",
		fooVal,
		dooVal,
		emptyVal,
		emptyVal,
		booVal,
	)

	expectedParsedArgs := kernelArgs(map[string]kernelArg{
		"foo": {
			position: 0,
			key:      "foo",
			value:    &fooVal,
		},
		"blah": {
			position: 1,
			key:      "blah",
			value:    nil,
		},
		"doo": {
			position: 2,
			key:      "doo",
			value:    &dooVal,
		},
		"huh": {
			position: 3,
			key:      "huh",
			value:    &emptyVal,
		},
		"bleh": {
			position: 4,
			key:      "bleh",
			value:    nil,
		},
		"duh": {
			position: 5,
			key:      "duh",
			value:    &emptyVal,
		},
		"boo": {
			position: 6,
			key:      "boo",
			value:    &booVal,
		},
	})

	actualParsedArgs := parseKernelArgs(argsString)
	require.Equal(t, expectedParsedArgs, actualParsedArgs, "kernel args parsed to unexpected values")

	reparsedArgs := parseKernelArgs(actualParsedArgs.String())
	require.Equal(t, expectedParsedArgs, reparsedArgs, "serializing and deserializing kernel args did not result in same value")
}
