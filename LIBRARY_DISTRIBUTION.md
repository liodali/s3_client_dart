# Native library distribution

The Dart build hook compiles `go_ffi` using Go and cgo for the matching desktop host and registers the resulting library as a Dart code asset. Dart bundles and resolves the asset; the package does not download executable code at runtime. The Go toolchain and a platform C compiler must be installed during builds, and Go modules must be available from cache or the network. Cross-compilation is not currently supported.

The release workflow also creates separate archives and a SHA-256 checksum file for the supported desktop targets. These are independent release artifacts; the Dart hook does not consume them. See [RELEASE_PROCESS.md](RELEASE_PROCESS.md).
