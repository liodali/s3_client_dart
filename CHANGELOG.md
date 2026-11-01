## Unreleased

- Build and bundle the Go library using a Dart build hook; Go and a C compiler are required when building.
- Give each Dart client an independent native client, propagate native errors, and list every page of results.
- Add desktop CI artifacts and release checksums.
- Breaking: remove runtime downloads, `LibraryDownloader`, `libraryPath`, and `autoDownload`; call `S3Client.close()` when finished.

## 0.1.5

- Previous package release.

## 0.1.0

- Initial Go FFI S3 client.
