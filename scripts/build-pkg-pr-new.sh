#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/.pkg-pr-new"

cd "$ROOT_DIR"

python3 scripts/fetch_meta.py

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/bin" "$OUT_DIR/scripts"

VERSION="$(node -p "require('./package.json').version")"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
SHA="$(git rev-parse --short HEAD)"
LDFLAGS="-s -w -X github.com/larksuite/cli/internal/build.Version=${VERSION}-${SHA} -X github.com/larksuite/cli/internal/build.Date=${DATE}"

build_target() {
  local goos="$1"
  local goarch="$2"
  local ext=""
  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
  fi

  local output="$OUT_DIR/bin/lark-cli-${goos}-${goarch}${ext}"
  echo "Building ${goos}/${goarch} -> ${output}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" -o "$output" ./main.go
}

build_target darwin amd64
build_target darwin arm64
build_target linux amd64
build_target linux arm64
build_target windows amd64
build_target windows arm64

cat > "$OUT_DIR/scripts/run.js" <<'RUNJS'
#!/usr/bin/env node
const path = require("path");
const { execFileSync } = require("child_process");

const isWindows = process.platform === "win32";

const platformMap = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const archMap = {
  x64: "amd64",
  arm64: "arm64",
};

const platform = platformMap[process.platform];
const arch = archMap[process.arch];

if (!platform || !arch) {
  console.error(`Unsupported platform: ${process.platform}-${process.arch}`);
  process.exit(1);
}

const ext = isWindows ? ".exe" : "";
const binary = path.join(__dirname, "..", "bin", `lark-cli-${platform}-${arch}${ext}`);

try {
  execFileSync(binary, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  process.exit(err.status || 1);
}
RUNJS

chmod +x "$OUT_DIR/scripts/run.js"

cat > "$OUT_DIR/package.json" <<EOF_JSON
{
  "name": "@larksuite/cli",
  "version": "${VERSION}-pr.${SHA}",
  "description": "The official CLI for Lark/Feishu open platform (PR preview build)",
  "bin": {
    "lark-cli": "scripts/run.js"
  },
  "repository": {
    "type": "git",
    "url": "git+https://github.com/larksuite/cli.git"
  },
  "license": "MIT",
  "files": [
    "bin",
    "scripts/run.js",
    "CHANGELOG.md",
    "LICENSE"
  ]
}
EOF_JSON

cp CHANGELOG.md "$OUT_DIR/CHANGELOG.md"
cp LICENSE "$OUT_DIR/LICENSE"

echo "Prepared pkg.pr.new package at $OUT_DIR"
