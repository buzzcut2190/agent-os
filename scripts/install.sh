#!/bin/sh
set -e

VERSION="${AGENTFS_VERSION:-0.2.0}"
BASE_URL="https://github.com/agent-os/agent-os/releases/download/v${VERSION}"

detect_os() {
    case "$(uname -s)" in Linux) echo "linux";; Darwin) echo "darwin";; *) echo "unsupported"; exit 1;; esac
}
detect_arch() {
    case "$(uname -m)" in x86_64|amd64) echo "amd64";; aarch64|arm64) echo "arm64";; *) echo "unsupported"; exit 1;; esac
}

OS=$(detect_os); ARCH=$(detect_arch)
INSTALL_DIR="${HOME}/.local/bin"
mkdir -p "$INSTALL_DIR"

echo "Installing agentfs v${VERSION} for ${OS}/${ARCH}..."

TMPDIR=$(mktemp -d)
trap 'rm -rf $TMPDIR' EXIT

curl -fsSL "${BASE_URL}/agentfs-${OS}-${ARCH}.tar.gz" -o "$TMPDIR/agentfs.tar.gz"
tar -xzf "$TMPDIR/agentfs.tar.gz" -C "$TMPDIR"
install -m 755 "$TMPDIR/agentfs-${OS}-${ARCH}/agentfs" "$INSTALL_DIR/agentfs"
install -m 755 "$TMPDIR/agentfs-mcp-${OS}-${ARCH}/agentfs-mcp" "$INSTALL_DIR/agentfs-mcp"

echo "agentfs installed to $INSTALL_DIR"
echo "Run 'agentfs version' to verify."
