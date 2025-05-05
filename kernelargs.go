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

import "strings"

// kernelArgs serializes+deserializes kernel boot parameters from/into a map.
// Kernel docs: https://www.kernel.org/doc/Documentation/admin-guide/kernel-parameters.txt
//
// "key=value" will result in map["key"] = &"value"
// "key=" will result in map["key"] = &""
// "key" will result in map["key"] = nil
type kernelArgs map[string]*string

const initSeparator = "--"
const initKey = "init"

// serialize the kernelArgs back to a string that can be provided
// to the kernel
func (kargs kernelArgs) String() string {
	var fields []string
	var initField string
	if initValue, ok := kargs[initKey]; ok && initValue != nil {
		initField = initKey + "=" + *initValue
	}
	for key, value := range kargs {
		if key == initKey {
			continue
		}
		field := key
		if value != nil {
			field += "=" + *value
		}
		fields = append(fields, field)
	}
	fields = append(fields, initField)
	return strings.Join(fields, " ")
}

// deserialize the provided string to a kernelArgs map
func parseKernelArgs(rawString string) kernelArgs {
	argMap := make(map[string]*string)
	fields := strings.Fields(rawString)
	passToInit := false

	for i := 0; i < len(fields); i++ {
		kvSplit := strings.SplitN(fields[i], "=", 2)
		key := kvSplit[0]
		if key == initSeparator {
			passToInit = true
		}

		var value *string
		if passToInit {
			key = initKey
			initValue := *argMap[key] + " " + kvSplit[0]
			value = &initValue
		} else if len(kvSplit) == 2 {
			value = &kvSplit[1]
		}

		argMap[key] = value
	}
	return argMap
}
