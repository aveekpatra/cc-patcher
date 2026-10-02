#!/bin/sh
# Install cc-patcher on macOS or Linux.
set -e
REPO=aveekpatra/cc-patcher
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac
url="https://github.com/$REPO/releases/latest/download/cc-patcher_${os}_${arch}.tar.gz"
dir=${INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$dir"
tmp=$(mktemp -d)
if command -v curl >/dev/null 2>&1; then curl -fsSL "$url" -o "$tmp/a.tgz"; else wget -qO "$tmp/a.tgz" "$url"; fi
tar -xzf "$tmp/a.tgz" -C "$tmp"
install -m 755 "$tmp/cc-patcher" "$dir/cc-patcher"
rm -rf "$tmp"
echo "installed to $dir/cc-patcher"
case ":$PATH:" in *":$dir:"*) ;; *) echo "add $dir to your PATH" ;; esac
