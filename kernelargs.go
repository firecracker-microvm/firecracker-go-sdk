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

// kernelArgs serializes+deserializes kernel boot parameters from/into a map.
// Kernel docs: https://www.kernel.org/doc/Documentation/admin-guide/kernel-parameters.txt
//
// "key=value" will result in map["key"] = &"value"
// "key=" will result in map["key"] = &""
// "key" will result in map["key"] = nil
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

type kernelArgs map[string]kernelArg

// serialize the sorted kernelArgs back to a string that can be provided
// to the kernel
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

func (kargs kernelArgs) Add(key string, value *string) {
	kargs[key] = kernelArg{
		position: uint(len(kargs)),
		key:      key,
		value:    value,
	}
}

// deserialize the provided string to a kernelArgs map
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
