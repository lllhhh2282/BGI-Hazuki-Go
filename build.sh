#!/usr/bin/env bash
set -euo pipefail

APP_NAME="hazuki"
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS="-s -w -X main.version=${VERSION}"

PLATFORMS=(
    "windows/amd64"
    "windows/arm64"
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
)

mkdir -p dist

for platform in "${PLATFORMS[@]}"; do
    GOOS="${platform%/*}"
    GOARCH="${platform#*/}"
    output="dist/${APP_NAME}-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        output="${output}.exe"
    fi

    echo "Building ${output}..."
    GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 go build \
        -ldflags "$LDFLAGS" \
        -o "$output" \
        ./cmd/hazuki
done

echo "Done."
ls -lh dist/
