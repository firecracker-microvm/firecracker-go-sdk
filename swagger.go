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

//go:generate find ./client ! -name swagger.yaml -type f -delete

// --skip-validation is used in the command-lines below to remove the network dependency that the swagger generator has
// in attempting to validate that the email address specified in the yaml file is valid.

//go:generate sh -c "docker run --add-host github.com:127.1.1.1 --rm -v $PWD:/work -w /work --entrypoint sh quay.io/goswagger/swagger:v0.33.1 -c 'apk add --no-cache go >/dev/null 2>&1 && swagger generate model -f ./client/swagger.yaml -T ./templates --model-package=client/models --copyright-file=COPYRIGHT_HEADER --skip-validation'"
//go:generate sh -c "docker run --add-host github.com:127.1.1.1 --rm -v $PWD:/work -w /work --entrypoint sh quay.io/goswagger/swagger:v0.33.1 -c 'apk add --no-cache go >/dev/null 2>&1 && swagger generate client -f ./client/swagger.yaml -T ./templates --model-package=client/models -c client --copyright-file=COPYRIGHT_HEADER --skip-validation -A Firecracker'"
//go:generate sh -c "docker run --add-host github.com:127.1.1.1 --rm -v $PWD:/work -w /work --entrypoint sh quay.io/goswagger/swagger:v0.33.1 -c 'apk add --no-cache go >/dev/null 2>&1 && swagger generate client -f ./client/swagger.yaml -C ./go_swagger_layout.yaml -T ./templates --model-package=client/models -c fctesting --copyright-file=COPYRIGHT_HEADER --skip-validation -A Firecracker'"

package firecracker
