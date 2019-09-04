#! /bin/bash
set -euo pipefail

download_safe () {
    url=$1
    checksum=$2
    dest=$3

    if [[ -f "${dest}" ]]; then
        sha256sum --check <<< "${checksum} ${dest}" && return 0
    fi

    temp=$(mktemp)
    curl --silent --show-error --retry 3 --max-time 30 --location \
         "${url}" \
         --output "${temp}"

    sha256sum --check <<< "${checksum} ${temp}"
    mv "${temp}" "${dest}"
}

download_safe \
    https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin \
    882fa465c43ab7d92e31bd4167da3ad6a82cb9230f9b0016176df597c6014cef \
    testdata/vmlinux

download_safe \
    https://github.com/firecracker-microvm/firecracker/releases/download/v0.17.0/jailer-v0.17.0 \
    f4ae19403f785a3687ecf7f831c101e7a54b5962e25ae2bbe5d97ef75fc292da \
    testdata/jailer
chmod +x testdata/jailer
