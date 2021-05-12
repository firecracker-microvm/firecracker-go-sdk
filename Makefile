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

# Set this to pass additional commandline flags to the go compiler, e.g. "make test EXTRAGOARGS=-v"
CARGO_CACHE_VOLUME_NAME?=firecracker-go-sdk--cargocache
DISABLE_ROOT_TESTS?=1
DOCKER_IMAGE_TAG?=latest
EXTRAGOARGS:=
FIRECRACKER_BUILDER_NAME=firecracker-builder
FIRECRACKER_TARGET?=x86_64-unknown-linux-musl

FC_TEST_DATA_PATH?=testdata
FIRECRACKER_BIN=$(FC_TEST_DATA_PATH)/firecracker-main
JAILER_BIN=$(FC_TEST_DATA_PATH)/jailer-main

UID = $(shell id -u)
GID = $(shell id -g)

firecracker_version=v0.24.2
arch=$(shell uname -m)

# The below files are needed and can be downloaded from the internet
release_url=https://github.com/firecracker-microvm/firecracker/releases/download/$(firecracker_version)/firecracker-$(firecracker_version)-$(arch).tgz

testdata_objects = \
$(FC_TEST_DATA_PATH)/vmlinux \
$(FC_TEST_DATA_PATH)/root-drive.img \
$(FC_TEST_DATA_PATH)/jailer \
$(FC_TEST_DATA_PATH)/firecracker \
$(FC_TEST_DATA_PATH)/ltag

testdata_dir = testdata/firecracker.tgz testdata/firecracker_spec-$(firecracker_version).yaml testdata/LICENSE testdata/NOTICE testdata/THIRD-PARTY

# --location is needed to follow redirects on github.com
curl = curl --location

all: build

test: all-tests

unit-tests: check-kvm $(testdata_objects)
	DISABLE_ROOT_TESTS=$(DISABLE_ROOT_TESTS) go test -short ./... $(EXTRAGOARGS)

all-tests: check-kvm $(testdata_objects)
	DISABLE_ROOT_TESTS=$(DISABLE_ROOT_TESTS) go test ./... $(EXTRAGOARGS)

check-kvm:
	@test -w /dev/kvm || \
		(echo "In order to run firecracker, $(shell whoami) must have write permission to /dev/kvm"; false)

generate build clean::
	go $@ $(EXTRAGOARGS)

clean::
	rm -fr build/

distclean: clean
	rm -rf $(testdata_objects)
	rm -rfv $(testdata_dir)
	docker volume rm -f $(CARGO_CACHE_VOLUME_NAME)

deps: $(testdata_objects)

$(FC_TEST_DATA_PATH)/vmlinux:
	$(curl) -o $@ https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/$(arch)/kernels/vmlinux.bin

$(FC_TEST_DATA_PATH)/firecracker $(FC_TEST_DATA_PATH)/jailer: $(FC_TEST_DATA_PATH)/fc.stamp

$(FC_TEST_DATA_PATH)/fc.stamp:
	$(curl) ${release_url} | tar -xvzf - -C $(FC_TEST_DATA_PATH)
	mv $(FC_TEST_DATA_PATH)/firecracker-$(firecracker_version)-$(arch) $(FC_TEST_DATA_PATH)/firecracker
	mv $(FC_TEST_DATA_PATH)/jailer-$(firecracker_version)-$(arch) $(FC_TEST_DATA_PATH)/jailer
	touch $@

$(FC_TEST_DATA_PATH)/root-drive.img:
	$(curl) -o $@ https://s3.amazonaws.com/spec.ccfc.min/img/hello/fsfiles/hello-rootfs.ext4

$(FC_TEST_DATA_PATH)/ltag:
	GO111MODULE=off GOBIN=$(abspath $(FC_TEST_DATA_PATH)) \
	go get github.com/kunalkushwaha/ltag

tools/firecracker-builder-stamp: tools/docker/Dockerfile
	docker build \
		-t localhost/$(FIRECRACKER_BUILDER_NAME):$(DOCKER_IMAGE_TAG) \
		-f tools/docker/Dockerfile \
		tools/docker
	touch $@

.PHONY: test-images
test-images: $(FIRECRACKER_BIN) $(JAILER_BIN)

$(FIRECRACKER_BIN) $(JAILER_BIN): tools/firecracker-builder-stamp
	mkdir -p build
	docker run --rm -it \
		--user $(UID):$(GID) \
		--volume $(CURDIR)/build:/artifacts \
		--volume $(CARGO_CACHE_VOLUME_NAME):/usr/local/cargo/registry \
		-e HOME=/tmp \
		--workdir=/firecracker \
		localhost/$(FIRECRACKER_BUILDER_NAME):$(DOCKER_IMAGE_TAG) \
		cargo build --release \
		--target-dir=/artifacts --target $(FIRECRACKER_TARGET) \
		-p firecracker -p jailer
	cp build/$(FIRECRACKER_TARGET)/release/firecracker $(FIRECRACKER_BIN)
	cp build/$(FIRECRACKER_TARGET)/release/jailer $(JAILER_BIN)

.PHONY: firecracker-clean
firecracker-clean:
	- docker run --rm -it \
		--user $(UID):$(GID) \
		--workdir /firecracker\
		localhost/$(FIRECRACKER_BUILDER_NAME):$(DOCKER_IMAGE_TAG) \
		cargo clean
	- rm $(FIRECRACKER_BIN) $(JAILER_BIN)

lint: deps
	gofmt -s -l .
	$(FC_TEST_DATA_PATH)/ltag -check -v -t .headers

.PHONY: all generate clean distclean build test unit-tests all-tests check-kvm
