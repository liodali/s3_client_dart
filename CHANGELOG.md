## Unreleased

- Add `copyObject` with cross-bucket sources, optional source version, and explicit metadata behavior. Oversized single-request copies throw `S3UnsupportedLargeCopyException`.
- Add `deleteObjects` with typed per-key successes and failures in `S3BatchDeleteResult`, chunked at 1000 keys.
- Add optional `versionId` to `headObject`, `download`, `deleteObject`, and `getPresignedUrl`, and add `listObjectVersionsPage` for providers with versioning.
- Add `uploadBytes` and `downloadBytes` for in-memory transfers with explicit size limits; oversized transfers throw `S3SizeLimitException`.

- Add `headObject`, returning `S3ObjectMetadata`; missing objects throw `S3NotFoundException` and permission failures throw `S3AccessDeniedException` (both extend `S3Exception`, which now exposes `code` and `statusCode`).
- Add `listObjectsPage` with prefix, delimiter, page size, and continuation token, returning `S3ListPage`. `listObjects()` is unchanged.
- Add an optional `S3UploadOptions` argument to `upload` for content type and user metadata. Existing calls are unaffected.
- Add `getPresignedPutUrl`, returning an `S3PresignedRequest` with the URL, method, and headers to send. `getPresignedUrl` (GET) is unchanged.
- `isKeyBucketExist` now also returns `false` for a bare 404 response with no error code.
- Empty object keys now throw `ArgumentError` in every key-based operation (`upload`, `download`, `deleteObject`, `getPresignedUrl`, `isKeyBucketExist`) instead of failing at the service.
- Any operation can now throw `S3NotFoundException` or `S3AccessDeniedException`; both extend `S3Exception`, so existing handlers keep working.
- Build and bundle the Go library using a Dart build hook; Go and a C compiler are required when building.
- Give each Dart client an independent native client, propagate native errors, and list every page of results.
- Add desktop CI artifacts and release checksums.
- Breaking: remove runtime downloads, `LibraryDownloader`, `libraryPath`, and `autoDownload`; call `S3Client.close()` when finished.

## 0.1.5

- Previous package release.

## 0.1.0

- Initial Go FFI S3 client.
