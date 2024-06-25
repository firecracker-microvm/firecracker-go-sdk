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
	"sort"
	"strings"
)

// kernelArg represents a key and optional value pair for passing an argument
// into the kernel. Additionally, it also saves the position of the argument
// in the whole command line input. This is important because the kernel stops reading
// everything after `--` and passes these keys into the init process
type kernelArg struct {
	position uint
	key      string
	value    *string
}

func (karg kernelArg) String() string {
	if karg.value != nil {
		return fmt.Sprintf("%s=%s", karg.key, *karg.value)
	}
	return karg.key
}

// kernelArgs serializes + deserializes kernel boot parameters from/into a map.
// Kernel docs: https://www.kernel.org/doc/Documentation/admin-guide/kernel-parameters.txt
//
// "key=value flag emptykey=" will be converted to
// map["key"] = { position: 0, key: "key", value: &"value" }
// map["flag"] = { position: 1, key: "flag", value: nil }
// map["emptykey"] = { position: 2, key: "emptykey", value: &"" }
type kernelArgs map[string]kernelArg

// Sorts the arguments by its position
// and serializes the map back into a single string
func (kargs kernelArgs) String() string {
	sortedArgs := make([]kernelArg, 0)
	for _, arg := range kargs {
		sortedArgs = append(sortedArgs, arg)
	}
	sort.SliceStable(sortedArgs, func(i, j int) bool {
		return sortedArgs[i].position < sortedArgs[j].position
	})

	args := make([]string, 0)
	for _, arg := range sortedArgs {
		args = append(args, arg.String())
	}
	return strings.Join(args, " ")
}

// Add a new kernel argument to the kernelArgs, also the position is saved
func (kargs kernelArgs) Add(key string, value *string) {
	kargs[key] = kernelArg{
		position: uint(len(kargs)),
		key:      key,
		value:    value,
	}
}

// Parses an input string and deserializes it into a map
// saving its position in the command line
func parseKernelArgs(rawString string) kernelArgs {
	args := make(map[string]kernelArg)
	for index, kv := range strings.Fields(rawString) {
		// only split into up to 2 fields (before and after the first "=")
		kvSplit := strings.SplitN(kv, "=", 2)

		key := kvSplit[0]
		var value *string
		if len(kvSplit) == 2 {
			value = &kvSplit[1]
		}

		args[key] = kernelArg{
			position: uint(index),
			key:      key,
			value:    value,
		}
	}

	return args
}
