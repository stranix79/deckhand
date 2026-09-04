#!/usr/bin/env bash
# Writes the Homebrew formula for a released tag into the stranix79/homebrew-tap
# checkout and pushes it. The formula builds from the tag's source tarball, so
# it needs no binary asset and no token in CI.
#
#   scripts/update-tap.sh v0.1.0 [path/to/homebrew-tap]
set -euo pipefail
tag="${1:?tag, e.g. v0.1.0}"
tap="${2:-$HOME/git/stranix/homebrew-tap}"
version="${tag#v}"
url="https://github.com/stranix79/deckhand/archive/refs/tags/${tag}.tar.gz"
sha=$(curl -sSL "$url" | shasum -a 256 | cut -d' ' -f1)
mkdir -p "$tap/Formula"
cat > "$tap/Formula/deckhand.rb" <<RUBY
class Deckhand < Formula
  desc "Turn a folder of HTML slides into a presentation: stage, phone remote, live audience"
  homepage "https://deckhand.stranix.net"
  url "${url}"
  sha256 "${sha}"
  license "MIT"
  head "https://github.com/stranix79/deckhand.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X github.com/stranix79/deckhand/internal/version.Version=#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags), "./cmd/deckhand"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/deckhand version")
  end
end
RUBY
echo "Formula/deckhand.rb written for ${tag} (sha256 ${sha})"
if [ -d "$tap/.git" ]; then
  git -C "$tap" add Formula/deckhand.rb
  git -C "$tap" commit -q -m "deckhand ${version}" && git -C "$tap" push -q && echo "pushed to tap"
fi
