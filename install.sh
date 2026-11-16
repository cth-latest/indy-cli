#!/bin/sh
# Installs the latest indy release: curl -fsSL https://raw.githubusercontent.com/cth-latest/indy-cli/main/install.sh | sh
set -eu
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "indy: unsupported architecture $arch" >&2; exit 1 ;;
esac
case "$os" in
  darwin | linux) ;;
  *) echo "indy: unsupported system $os" >&2; exit 1 ;;
esac
dir=${INDY_INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$dir"
curl -fsSL "https://github.com/cth-latest/indy-cli/releases/latest/download/indy-$os-$arch" -o "$dir/indy.new"
chmod +x "$dir/indy.new"
mv "$dir/indy.new" "$dir/indy"
echo "Installed $("$dir/indy" version) to $dir/indy"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "Add $dir to your PATH to run it as 'indy'." ;;
esac
