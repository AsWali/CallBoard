#!/bin/sh
set -eu

repo="AsWali/CallBoard"
dir="${CALLBOARD_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "✗ this script is for macOS and Linux; on Windows, download callboard from https://github.com/$repo/releases" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64 | aarch64) arch=arm64 ;;
  x86_64 | amd64) arch=amd64 ;;
  *) echo "✗ no build for $(uname -m); build it yourself: go install github.com/$repo/cmd/callboard@latest" >&2; exit 1 ;;
esac

tag="${CALLBOARD_VERSION:-}"
if [ -z "$tag" ]; then
  tag="$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
fi
[ -n "$tag" ] || { echo "✗ couldn't find the latest release of $repo" >&2; exit 1; }
v="${tag#v}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
name="callboard-$v-$os-$arch"
url="https://github.com/$repo/releases/download/$tag"

echo "· downloading callboard $v for $os/$arch"
curl -fsSL -o "$tmp/$name" "$url/$name"
curl -fsSL -o "$tmp/sums" "$url/callboard-$v-sha256.txt"
want="$(grep " $name\$" "$tmp/sums" | cut -d' ' -f1)"
if command -v shasum >/dev/null 2>&1; then have="$(shasum -a 256 "$tmp/$name" | cut -d' ' -f1)"; else have="$(sha256sum "$tmp/$name" | cut -d' ' -f1)"; fi
[ -n "$want" ] && [ "$want" = "$have" ] || { echo "✗ the download doesn't match its checksum; nothing was installed" >&2; exit 1; }
echo "✓ downloaded and checked"

mkdir -p "$dir"
chmod +x "$tmp/$name"
mv "$tmp/$name" "$dir/callboard"
echo "✓ installed $dir/callboard"

case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "! $dir isn't on your PATH yet. Add this to your shell profile: export PATH=\"$dir:\$PATH\"" ;;
esac
echo
echo "Next: callboard setup --global   (connects Claude Code, Codex and git for every project)"
