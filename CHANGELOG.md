## Unreleased

- Add `headObject`, returning `S3ObjectMetadata`; missing objects throw `S3NotFoundException` and permission failures throw `S3AccessDeniedException` (both extend `S3Exception`, which now exposes `code` and `statusCode`).
- Add `listObjectsPage` with prefix, delimiter, page size, and continuation token, returning `S3ListPage`. `listObjects()` is unchanged.
- Add an optional `S3UploadOptions` argument to `upload` for content type and user metadata. Existing calls are unaffected.
- Add `getPresignedPutUrl`, returning an `S3PresignedRequest` with the URL, method, and headers to send. `getPresignedUrl` (GET) is unchanged.

- Build and bundle the Go library using a Dart build hook; Go and a C compiler are required when building.
- Give each Dart client an independent native client, propagate native errors, and list every page of results.
- Add desktop CI artifacts and release checksums.
- Breaking: remove runtime downloads, `LibraryDownloader`, `libraryPath`, and `autoDownload`; call `S3Client.close()` when finished.

## 0.1.5

- Previous package release.

## 0.1.0

- Initial Go FFI S3 client.
