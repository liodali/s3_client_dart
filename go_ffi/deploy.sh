#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
case "$(go env GOOS)" in
  darwin) output=darwin/s3_client_dart.dylib ;;
  linux) output=linux/s3_client_dart.so ;;
  windows) output=windows/s3_client_dart.dll ;;
  *) echo 'Unsupported Go target' >&2; exit 1 ;;
esac
mkdir -p "$(dirname "$output")"
CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -o "$output" .
