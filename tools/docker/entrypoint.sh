#!/bin/bash

cargo build --release --target $@
cp build/cargo_target/x86_64-unknown-linux-musl/release/firecracker /artifacts/firecracker-master
cp build/cargo_target/x86_64-unknown-linux-musl/release/jailer /artifacts/jailer-master
