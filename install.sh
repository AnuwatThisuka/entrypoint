#!/bin/sh
# entrypoint installer — downloads the right prebuilt binary for your OS/arch
# from GitHub Releases, verifies its checksum, and installs it. No Node, Bun,
# or node_modules required — entrypoint is a single static binary.
#
#   curl -fsSL https://raw.githubusercontent.com/AnuwatThisuka/entrypoint/main/install.sh | sh
#
# Overrides (env vars):
#   ENTRYPOINT_VERSION=v0.1.0   install a specific tag (default: latest)
#   ENTRYPOINT_BINDIR=~/bin     install location (default: /usr/local/bin,
#                               falling back to ~/.local/bin if not writable)
set -eu

REPO="AnuwatThisuka/entrypoint"
BINARY="entrypoint"

log()  { printf '%s\n' "$*" >&2; }
die()  { log "error: $*"; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

# --- detect platform -------------------------------------------------------
os=$(uname -s)
case "$os" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS: $os (prebuilt binaries cover Linux and macOS; build from source with \`go install $REPO/cmd/$BINARY@latest\`)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

# --- pick a downloader -----------------------------------------------------
if have curl; then
  dl() { curl -fsSL "$1"; }
  dlo() { curl -fsSL -o "$2" "$1"; }
elif have wget; then
  dl() { wget -qO- "$1"; }
  dlo() { wget -qO "$2" "$1"; }
else
  die "need curl or wget"
fi

# --- resolve version -------------------------------------------------------
tag="${ENTRYPOINT_VERSION:-}"
if [ -z "$tag" ]; then
  log "resolving latest release..."
  # Parse tag_name from the GitHub API without needing jq.
  tag=$(dl "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -n1 | cut -d '"' -f4)
  [ -n "$tag" ] || die "could not determine latest version — set ENTRYPOINT_VERSION"
fi
version="${tag#v}" # strip leading v for the archive name

archive="${BINARY}_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

# --- download + verify -----------------------------------------------------
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

log "downloading $archive ($tag)..."
dlo "$base/$archive" "$tmp/$archive" || die "download failed: $base/$archive"

if dlo "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
  log "verifying checksum..."
  expected=$(grep " ${archive}\$" "$tmp/checksums.txt" | awk '{print $1}')
  [ -n "$expected" ] || die "no checksum listed for $archive"
  if have sha256sum; then
    actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
  elif have shasum; then
    actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
  else
    actual=""; log "warning: no sha256 tool found — skipping verification"
  fi
  if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
    die "checksum mismatch — expected $expected, got $actual"
  fi
else
  log "warning: checksums.txt not found — skipping verification"
fi

tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/$BINARY" ] || die "archive did not contain a '$BINARY' binary"
chmod +x "$tmp/$BINARY"

# --- install ---------------------------------------------------------------
bindir="${ENTRYPOINT_BINDIR:-/usr/local/bin}"
install_to() { mkdir -p "$1" 2>/dev/null && mv "$tmp/$BINARY" "$1/$BINARY" 2>/dev/null; }

if install_to "$bindir"; then
  :
elif [ -z "${ENTRYPOINT_BINDIR:-}" ] && install_to "$HOME/.local/bin"; then
  bindir="$HOME/.local/bin"
elif have sudo && sudo sh -c "mkdir -p '$bindir' && mv '$tmp/$BINARY' '$bindir/$BINARY'"; then
  :
else
  die "could not write to $bindir — set ENTRYPOINT_BINDIR to a writable dir"
fi

log ""
log "installed $BINARY $tag -> $bindir/$BINARY"
case ":$PATH:" in
  *":$bindir:"*) log "run: $BINARY --version" ;;
  *) log "note: $bindir is not on your PATH. Add it:"
     log "  export PATH=\"$bindir:\$PATH\"" ;;
esac
