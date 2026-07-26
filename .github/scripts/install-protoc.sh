#!/usr/bin/env bash
# Install the official protoc binary from protocolbuffers/protobuf releases.
# Env:
#   PROTOC_VERSION         - version without leading "v" (default: 35.1)
#   PROTOC_INSTALL_PREFIX  - install root (default: $HOME/.local/protoc)
# On GitHub Actions, the bin dir is appended to $GITHUB_PATH.
set -euo pipefail

PROTOC_VERSION="${PROTOC_VERSION:-35.1}"
INSTALL_PREFIX="${PROTOC_INSTALL_PREFIX:-${HOME}/.local/protoc}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "${os}" in
  linux)  protoc_os="linux" ;;
  darwin) protoc_os="osx" ;;
  *)
    echo "install-protoc: unsupported OS: ${os}" >&2
    exit 1
    ;;
esac

case "${arch}" in
  x86_64|amd64)  protoc_arch="x86_64" ;;
  aarch64|arm64) protoc_arch="aarch_64" ;;
  *)
    echo "install-protoc: unsupported arch: ${arch}" >&2
    exit 1
    ;;
esac

asset="protoc-${PROTOC_VERSION}-${protoc_os}-${protoc_arch}.zip"
url="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${asset}"

echo "install-protoc: downloading ${url}"
tmpdir="$(mktemp -d)"
cleanup() { rm -rf "${tmpdir}"; }
trap cleanup EXIT

if ! curl -fsSL --retry 3 --retry-delay 2 -o "${tmpdir}/protoc.zip" "${url}"; then
  if [[ "${protoc_os}" == "osx" ]]; then
    asset="protoc-${PROTOC_VERSION}-osx-universal_binary.zip"
    url="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${asset}"
    echo "install-protoc: retrying with ${url}"
    curl -fsSL --retry 3 --retry-delay 2 -o "${tmpdir}/protoc.zip" "${url}"
  else
    echo "install-protoc: download failed for ${url}" >&2
    exit 1
  fi
fi

mkdir -p "${INSTALL_PREFIX}"
unzip -qo "${tmpdir}/protoc.zip" -d "${INSTALL_PREFIX}"

bin_dir="${INSTALL_PREFIX}/bin"
if [[ ! -x "${bin_dir}/protoc" ]]; then
  echo "install-protoc: protoc binary missing at ${bin_dir}/protoc" >&2
  exit 1
fi

if [[ -n "${GITHUB_PATH:-}" ]]; then
  echo "${bin_dir}" >> "${GITHUB_PATH}"
fi
export PATH="${bin_dir}:${PATH}"

echo "install-protoc: installed $(command -v protoc)"
protoc --version
