# Building

Install Dart 3.11 or newer, Go 1.25.5 or newer, and a C compiler for cgo. Then run `dart pub get`, `dart analyze --fatal-infos`, and `dart test`. The Dart build hook builds the Go code for the matching host and bundles it automatically. Go dependencies must be available in the module cache or downloadable. See [README.md](README.md) for supported targets and [RELEASE_PROCESS.md](RELEASE_PROCESS.md) for CI artifacts.
