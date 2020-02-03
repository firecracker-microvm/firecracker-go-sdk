#!/bin/bash

cargo build --release --target-dir=/artifacts --target $@
cp /artifacts/x86_64-unknown-linux-musl/release/firecracker /artifacts/firecracker-master
cp /artifacts/x86_64-unknown-linux-musl/release/jailer /artifacts/jailer-master
