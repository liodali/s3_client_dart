# Release process

Update `version` in `pubspec.yaml` and `CHANGELOG.md`, then run `dart analyze --fatal-infos`, `dart test`, and `go test ./...` from `go_ffi`. The Dart tests require Go 1.25.5+, a C toolchain, and access to the Go dependencies or a populated module cache.

Push a tag named `s3-client-dart-v<version>`, matching `pubspec.yaml`. CI tests the build hook and creates native libraries on native runners for macOS x64/arm64, Linux x64/arm64, and Windows x64. It uploads workflow artifacts and, for matching tags, attaches archives and `SHA256SUMS` to a GitHub release. A manual workflow run produces workflow artifacts but does not create a release. Check the workflow and validate the artifacts on each target before publishing.

Run `dart pub publish --dry-run` before publishing the Dart package. Choose and add the project license before making the repository public. The Dart package compiles its native code from source, so a release artifact is not required by the build hook.
