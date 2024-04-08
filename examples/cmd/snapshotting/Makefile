# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
# 	http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

RELEASE_URL=https://github.com/firecracker-microvm/firecracker/releases
VER=v1.4.1

ARCH=$(shell uname -m)
GID = $(shell id -g)

PWD=$(shell pwd)
GOBIN=$(PWD)/bin
FC_TEST_DATA_PATH?=$(PWD)

# --location is needed to follow redirects on github.com
curl = curl --location

GO_VERSION = $(shell go version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f1,2)
ifeq ($(GO_VERSION), $(filter $(GO_VERSION),1.14 1.15))
    define install_go
		cd .hack; GO111MODULE=on GOBIN=$(GOBIN) go get $(1)@$(2) 
		cd .hack; GO111MODULE=on GOBIN=$(GOBIN) go install $(1)
    endef
else
    define install_go
		GOBIN=$(GOBIN) go install $(1)@$(2)
    endef
endif

all: snapshot-example plugins root-drive-with-ssh.img root-drive-ssh-key vmlinux firecracker

plugins: bin/tc-redirect-tap bin/ptp bin/host-local | bin

bin:
	mkdir -p bin

bin/tc-redirect-tap: bin
	$(call install_go,github.com/awslabs/tc-redirect-tap/cmd/tc-redirect-tap,v0.0.0-20240408144842-496fddc89db6)

bin/ptp: bin
	$(call install_go,github.com/containernetworking/plugins/plugins/main/ptp,v1.1.1)

bin/host-local: bin
	$(call install_go,github.com/containernetworking/plugins/plugins/ipam/host-local,v1.1.1)

vmlinux:
	$(curl) -o vmlinux https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/${ARCH}/kernels/vmlinux.bin

firecracker:
	$(curl) ${RELEASE_URL}/download/${VER}/firecracker-${VER}-${ARCH}.tgz | tar -xz
	mv release-${VER}-${ARCH}/firecracker-${VER}-${ARCH} firecracker
	rm -rf release-${VER}-${ARCH}

# Use rootfs and ssh key pre-built from S3 following firecracker getting-started guide
# https://github.com/firecracker-microvm/firecracker/blob/main/docs/getting-started.md
# TODO: Change the pre-built rootfs to ubuntu 22.04 once firecracker team has that in the public S3 bucket
# Currently the S3 bucket only has ubuntu 18.04 image
root-drive-with-ssh.img root-drive-ssh-key:
	$(curl) -o root-drive-with-ssh.img https://s3.amazonaws.com/spec.ccfc.min/ci-artifacts/disks/${ARCH}/ubuntu-18.04.ext4
	$(curl) -o root-drive-ssh-key https://s3.amazonaws.com/spec.ccfc.min/ci-artifacts/disks/${ARCH}/ubuntu-18.04.id_rsa
	
	
snapshot-example:
	go build -o $@

run: snapshot-example
	./snapshot-example

clean:
	rm -rf bin firecracker root-drive-ssh-key root-drive-with-ssh.img snapshot-example vmlinux  

.PHONY: all clean plugins run
