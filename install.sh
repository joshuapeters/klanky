#!/usr/bin/env bash
# Klanky installer.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/joshuapeters/klanky/main/install.sh | bash
#
# Optional environment variables:
#   KLANKY_VERSION       Pin a specific version (e.g. "0.1.0" or "v0.1.0"). Default: latest release.
#   KLANKY_INSTALL_DIR   Directory to install the binary into. Default: /usr/local/bin.

set -euo pipefail

REPO="joshuapeters/klanky"
INSTALL_DIR="${KLANKY_INSTALL_DIR:-/usr/local/bin}"
VERSION="${KLANKY_VERSION:-}"

if [ -t 1 ]; then
  RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; BOLD=$'\033[1m'; RESET=$'\033[0m'
else
  RED=""; GREEN=""; YELLOW=""; BOLD=""; RESET=""
fi

info()  { printf '%s\n' "$*"; }
warn()  { printf '%s%s%s\n' "$YELLOW" "$*" "$RESET" >&2; }
error() { printf '%s%s%s\n' "$RED" "$*" "$RESET" >&2; }
die()   { error "$*"; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require curl
require tar
require uname
require mktemp
require awk

if command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  SHA_CMD=(shasum -a 256)
else
  die "need sha256sum or shasum to verify the download"
fi

# Detect OS.
case "$(uname -s)" in
  Darwin) OS="darwin" ;;
  Linux)  OS="linux" ;;
  MINGW*|MSYS*|CYGWIN*)
    die "Windows is not supported. Use WSL, or build from source: https://github.com/${REPO}#from-source"
    ;;
  *)
    die "unsupported OS: $(uname -s). Klanky publishes binaries for darwin and linux only."
    ;;
esac

# Detect arch.
case "$(uname -m)" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    die "unsupported architecture: $(uname -m). Klanky publishes binaries for amd64 and arm64 only."
    ;;
esac

# Resolve version (latest by default).
if [ -z "$VERSION" ]; then
  info "Resolving latest version..."
  resolved_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest")" \
    || die "failed to resolve latest release URL"
  VERSION="${resolved_url##*/}"
  [ -n "$VERSION" ] || die "could not parse version from $resolved_url"
fi
VERSION_NUM="${VERSION#v}"
TAG="v${VERSION_NUM}"

ASSET="klanky_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
TARBALL_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"

info "Installing ${BOLD}klanky ${TAG}${RESET} (${OS}/${ARCH}) to ${BOLD}${INSTALL_DIR}${RESET}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

info "Downloading ${ASSET}..."
curl -fsSL "$TARBALL_URL"   -o "$TMPDIR/$ASSET"        || die "download failed: $TARBALL_URL"
curl -fsSL "$CHECKSUMS_URL" -o "$TMPDIR/checksums.txt" || die "download failed: $CHECKSUMS_URL"

# Verify checksum. checksums.txt format: "<sha256>  <filename>", one per line.
expected="$(awk -v f="$ASSET" '$2 == f { print $1 }' "$TMPDIR/checksums.txt")"
[ -n "$expected" ] || die "no checksum entry for $ASSET in checksums.txt"
actual="$("${SHA_CMD[@]}" "$TMPDIR/$ASSET" | awk '{print $1}')"
if [ "$expected" != "$actual" ]; then
  die "checksum mismatch for $ASSET: expected $expected, got $actual"
fi
info "Checksum ok."

# Extract just the klanky binary.
tar -xzf "$TMPDIR/$ASSET" -C "$TMPDIR" klanky || die "failed to extract klanky from $ASSET"
chmod +x "$TMPDIR/klanky"

# Install. Try without sudo, escalate if needed.
mkdir -p "$INSTALL_DIR" 2>/dev/null || true
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMPDIR/klanky" "$INSTALL_DIR/klanky"
elif command -v sudo >/dev/null 2>&1; then
  warn "$INSTALL_DIR is not writable; using sudo."
  sudo mv "$TMPDIR/klanky" "$INSTALL_DIR/klanky"
else
  die "$INSTALL_DIR is not writable and sudo is not available. Set KLANKY_INSTALL_DIR to a writable directory."
fi

installed="${INSTALL_DIR%/}/klanky"
[ -x "$installed" ] || die "install failed: $installed not executable after move"

case ":$PATH:" in
  *":${INSTALL_DIR%/}:"*) ;;
  *)
    warn "$INSTALL_DIR is not on your PATH. Add it to your shell rc, e.g.:"
    warn "  export PATH=\"${INSTALL_DIR%/}:\$PATH\""
    ;;
esac

printf '%s%s✓%s installed klanky %s to %s\n' "$BOLD" "$GREEN" "$RESET" "$TAG" "$installed"

if "$installed" version >/dev/null 2>&1; then
  "$installed" version | sed 's/^/  /'
fi
