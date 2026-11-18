#!/bin/sh
# Deckhand installer for macOS and Linux.
#
#   curl -fsSL https://deckhand.show/install.sh | sh
#
# It downloads the release binary for this machine from GitHub, checks its
# SHA-256 against the release checksums, and puts `deckhand` in a directory
# of your PATH. No sudo, nothing else touched.
#
#   DECKHAND_VERSION=1.5.0       install that version instead of the latest
#   DECKHAND_INSTALL_DIR=~/bin   install there instead of the default
#
# Read it before you pipe it: https://deckhand.show/install.sh
set -eu

REPO="stranix79/deckhand"

say() { printf '%s\n' "$*"; }
die() { printf 'deckhand install: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }

# Everything runs from main, called on the last line: a download cut in the
# middle runs nothing at all.
main() {
  need curl
  need tar
  need uname

  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) die "unsupported system $(uname -s). Windows: get the zip on https://deckhand.show/download" ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) die "unsupported processor $(uname -m). Build from source: https://github.com/$REPO" ;;
  esac

  version="${DECKHAND_VERSION:-}"
  if [ -z "$version" ]; then
    # /releases/latest redirects to /releases/tag/vX.Y.Z: no API, no token.
    latest=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") ||
      die "cannot reach GitHub to find the latest version"
    version="${latest##*/}"
  fi
  version="${version#v}"
  case "$version" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *) die "unexpected version \"$version\"" ;;
  esac

  asset="deckhand_${version}_${os}_${arch}.tar.gz"
  base="https://github.com/$REPO/releases/download/v${version}"
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  say "Deckhand $version for $os/$arch"
  curl -fsSL -o "$tmp/$asset" "$base/$asset" || die "download failed: $base/$asset"
  curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || die "download failed: $base/checksums.txt"

  want=$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")
  [ -n "$want" ] || die "$asset is not listed in checksums.txt"
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
  elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
  else
    die "sha256sum or shasum is required to check the download"
  fi
  [ "$want" = "$got" ] || die "checksum mismatch for $asset: expected $want, got $got"

  tar -xzf "$tmp/$asset" -C "$tmp" deckhand

  dir="${DECKHAND_INSTALL_DIR:-}"
  if [ -z "$dir" ]; then
    if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
      dir=/usr/local/bin
    else
      dir="$HOME/.local/bin"
    fi
  fi
  mkdir -p "$dir" || die "cannot create $dir"
  # Copy then rename: a running deckhand is never overwritten in place.
  cp "$tmp/deckhand" "$dir/.deckhand.new" || die "cannot write to $dir (set DECKHAND_INSTALL_DIR)"
  chmod 755 "$dir/.deckhand.new"
  mv -f "$dir/.deckhand.new" "$dir/deckhand"

  say "Installed $dir/deckhand ($("$dir/deckhand" version))"
  case ":$PATH:" in
    *":$dir:"*) ;;
    *)
      say ""
      say "$dir is not in your PATH. Add this line to your shell profile:"
      say "  export PATH=\"$dir:\$PATH\""
      ;;
  esac
  say ""
  say "Next: deckhand present <folder>      (or: deckhand import talk.pptx)"
}

main "$@"
