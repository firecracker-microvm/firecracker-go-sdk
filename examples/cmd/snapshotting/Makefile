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
VER=v1.0.0

ARCH=$(shell uname -m)
GID = $(shell id -g)

PWD=$(shell pwd)
GOBIN=$(PWD)/bin
FC_TEST_DATA_PATH?=$(PWD)

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

all: plugins image vmlinux firecracker

plugins: bin/tc-redirect-tap bin/ptp bin/host-local | bin

bin:
	mkdir -p bin

bin/tc-redirect-tap: bin
	$(call install_go,github.com/awslabs/tc-redirect-tap/cmd/tc-redirect-tap,v0.0.0-20220715050423-f2af44521093)

bin/ptp: bin
	$(call install_go,github.com/containernetworking/plugins/plugins/main/ptp,v1.1.1)

bin/host-local: bin
	$(call install_go,github.com/containernetworking/plugins/plugins/ipam/host-local,v1.1.1)
	
image:
ifeq ($(GID), 0)
	- cp ${FC_TEST_DATA_PATH}/root-drive-with-ssh.img root-drive-with-ssh.img
	- cp ${FC_TEST_DATA_PATH}/root-drive-ssh-key root-drive-ssh-key
	$(MAKE) root-drive-with-ssh.img root-drive-ssh-key
else
	$(error unable to place ssh key without root permissions)
endif

vmlinux:
	curl --location -o vmlinux https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/${ARCH}/kernels/vmlinux.bin

firecracker:
	curl -L ${RELEASE_URL}/download/${VER}/firecracker-${VER}-${ARCH}.tgz | tar -xz
	mv release-${VER}-${ARCH}/firecracker-${VER}-${ARCH} firecracker
	rm -rf release-${VER}-${ARCH}

root-drive-with-ssh.img root-drive-ssh-key:
	- mkdir temp
	- git clone https://github.com/firecracker-microvm/firecracker temp
	temp/tools/devtool build_rootfs
	cp temp/build/rootfs/bionic.rootfs.ext4 root-drive-with-ssh.img
	cp temp/build/rootfs/ssh/id_rsa root-drive-ssh-key
	rm -rf temp

run: all
	go run example_demo.go

clean:
	rm -rf bin firecracker root-drive-ssh-key root-drive-with-ssh.img vmlinux

.PHONY: all clean image plugins run
