# Go native library

The Dart build hook builds this module as a cgo shared library. Install Go 1.25.5 or newer and a platform C compiler. Run `go test ./...` to check the module, or run `./deploy.sh` to build a library for the host. The generated C ABI is internal to the Dart package.
