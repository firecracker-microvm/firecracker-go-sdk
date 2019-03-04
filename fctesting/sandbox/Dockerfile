ARG TMP_BUILD_DIR=/tmp/build

FROM rust:1.32 as build

ARG TMP_BUILD_DIR
ARG FIRECRACKER_SRC_DIR="/firecracker"
ARG FIRECRACKER_BUILD_DIR="$FIRECRACKER_SRC_DIR/build"
ARG CARGO_REGISTRY_DIR="$FIRECRACKER_BUILD_DIR/cargo_registry"
ARG FIRECRACKER_VERSION="0.15.0"

# Install system dependecies
#
RUN apt-get update \
    && apt-get -y install --no-install-recommends \
        musl-tools

# Add musl to rust std
RUN mkdir "$TMP_BUILD_DIR" \
    	&& rustup install 1.32.0 \
	&& rustup override set 1.32.0 \
        && rustup target add x86_64-unknown-linux-musl \
    && cd /

# Install Firecracker and set binaries to TMP_BUILD_DIR
RUN git clone https://github.com/firecracker-microvm/firecracker.git
WORKDIR firecracker
RUN git checkout tags/v"$FIRECRACKER_VERSION"
RUN cargo build --release --features vsock --target-dir "$TMP_BUILD_DIR"
RUN cp "$TMP_BUILD_DIR"/x86_64-unknown-linux-musl/release/firecracker "$TMP_BUILD_DIR"
RUN cp "$TMP_BUILD_DIR"/x86_64-unknown-linux-musl/release/jailer "$TMP_BUILD_DIR"

FROM golang:1.12

ARG TMP_BUILD_DIR
ENV FC_TEST_DATA_PATH="$TMP_BUILD_DIR"
ARG FIRECRACKER_GO_SDK_SRC_DIR="/firecracker-go-sdk"
ADD . /firecracker-go-sdk/
COPY --from=build "$TMP_BUILD_DIR" "$TMP_BUILD_DIR"

WORKDIR "$FIRECRACKER_GO_SDK_SRC_DIR"
RUN cp -rf testdata/* "$TMP_BUILD_DIR"
RUN curl -fsSL -o "$TMP_BUILD_DIR"/vmlinux https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin
RUN go mod download
CMD ["make", "test"]
