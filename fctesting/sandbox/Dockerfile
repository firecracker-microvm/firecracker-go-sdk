FROM rust:1.32 as firecracker-build

ENV TMP_BUILD_DIR="/build"
ENV FIRECRACKER_SRC_DIR="/firecracker"
ENV FIRECRACKER_BUILD_DIR="$FIRECRACKER_SRC_DIR/build"
ENV CARGO_REGISTRY_DIR="$FIRECRACKER_BUILD_DIR/cargo_registry"
ARG FIRECRACKER_VERSION="0.15.0"

# Install system dependecies
#
RUN apt-get update \
    && apt-get -y install --no-install-recommends \
        musl-tools

# Add musl to rust std
RUN mkdir "$TMP_BUILD_DIR" \
        && rustup target add x86_64-unknown-linux-musl

# Install Firecracker and set binaries to TMP_BUILD_DIR
RUN git clone https://github.com/firecracker-microvm/firecracker.git \
        && cd firecracker \
        && git checkout tags/v"$FIRECRACKER_VERSION" \
        && cargo build --release --features vsock --target-dir "$TMP_BUILD_DIR" \
        && cp "$TMP_BUILD_DIR"/x86_64-unknown-linux-musl/release/firecracker "$TMP_BUILD_DIR" \
        && cp "$TMP_BUILD_DIR"/x86_64-unknown-linux-musl/release/jailer "$TMP_BUILD_DIR"

FROM golang:1.12

ENV TMP_BUILD_DIR="/build"
ENV FC_TEST_DATA_PATH="$TMP_BUILD_DIR"
ARG FIRECRACKER_GO_SDK_SRC_DIR="/firecracker-go-sdk"
COPY --from=firecracker-build "$TMP_BUILD_DIR" "$TMP_BUILD_DIR"

WORKDIR "$FIRECRACKER_GO_SDK_SRC_DIR"
COPY ["./testdata/", "$TMP_BUILD_DIR"]
COPY ["go.mod", "go.sum", "/firecracker-go-sdk/"]
RUN curl -fsSL -o "$TMP_BUILD_DIR"/vmlinux https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin \
        && go mod download
COPY [".", "/firecracker-go-sdk/"]
CMD ["make", "test"]
