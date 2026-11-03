# S3 Client for Dart

A Dart client for Amazon S3 and S3-compatible storage, backed by the AWS SDK for Go through Dart FFI.

## Requirements

- Dart 3.11 or newer.
- Go 1.25.5 or newer and a C compiler suitable for cgo on the build machine.
- A supported desktop target: macOS or Linux on x64 or arm64, or Windows on x64. The build hook requires a matching host; cross-compilation, mobile, and web are not supported. Flutter support has not yet been validated.

The Dart build hook compiles the Go source and bundles the native library. The first build can take several minutes and may download Go modules. Pre-populate the Go module cache for offline builds. No runtime download or manual library path is required.

## Installation

Once the package is published, add it to your project:

```sh
dart pub add s3_client_dart
```

For local development before publication, use a path dependency in your application's `pubspec.yaml`:

```yaml
dependencies:
  s3_client_dart:
    path: ../s3_client_dart
```

Adjust the path to your checkout.

## Quick start

```dart
import 'dart:io';

import 'package:s3_client_dart/s3_client_dart.dart';

Future<void> main() async {
  final client = S3Client();
  try {
    client.initialize(
      configuration: S3Configuration(
        endpoint: 'https://s3.amazonaws.com',
        bucketName: 'example-bucket',
        accessKeyId: Platform.environment['AWS_ACCESS_KEY_ID']!,
        secretAccessKey: Platform.environment['AWS_SECRET_ACCESS_KEY']!,
        sessionToken: Platform.environment['AWS_SESSION_TOKEN'] ?? '',
        accountId: '',
        region: 'us-east-1',
      ),
    );

    final keys = await client.listObjects();
    for (final key in keys) {
      print(key);
    }
  } on S3Exception catch (error) {
    print('S3 request failed: ${error.message}');
  } finally {
    client.close();
  }
}
```

Supply credentials through environment variables or a secrets manager. The example expects `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` to be set.

## Configuration

All `S3Configuration` arguments are required strings. Use an empty string for fields you do not need.

| Field | Description |
| --- | --- |
| `endpoint` | S3 service endpoint, such as `https://s3.amazonaws.com`. Pass an empty string to use the AWS SDK's regional endpoint. |
| `bucketName` | Bucket used by this client. |
| `accessKeyId` | Access key ID. |
| `secretAccessKey` | Secret access key. |
| `sessionToken` | Session token for temporary credentials, or `''` when not needed. |
| `accountId` | Account ID when needed by the provider, or `''`. |
| `region` | Service region, such as `us-east-1`. |

The client uses the supplied static credentials and path-style addressing. For a Cloudflare R2 configuration example, see [CLOUDFLARE_R2_SETUP.md](CLOUDFLARE_R2_SETUP.md).

## API

Create a client with `S3Client()` and call `initialize(configuration: ...)` before making requests. Each client has its own configuration. Call `close()` when finished; calling it more than once is safe, and a closed client can be initialized again.

| Method | Result |
| --- | --- |
| `upload(filePath, objectKey, options: S3UploadOptions())` | Uploads a local file and returns its object key as `Future<String>`. `options` optionally sets the content type and user metadata. |
| `listObjects()` | Returns all object keys as `Future<List<String>>`, following pagination. |
| `listObjectsPage(prefix:, delimiter:, pageSize:, continuationToken:)` | Returns one `S3ListPage` with `objects` (`S3ObjectSummary`), `commonPrefixes`, and `nextContinuationToken` (`null` on the last page). `pageSize` must be 1 to 1000. |
| `headObject(objectKey, versionId:)` | Returns `S3ObjectMetadata` (content length, content type, ETag, last-modified time, user metadata, version ID) without downloading the body. Values the service omits are `null`. |
| `copyObject(sourceKey, destinationKey, sourceBucket:, sourceVersionId:, metadataBehavior:, options:)` | Copies an object and returns an `S3CopyResult`. Metadata is kept by default; use `S3CopyMetadataBehavior.replace` with `options` to replace it. |
| `deleteObjects(objects)` | Deletes many `S3ObjectIdentifier` values (key plus optional version ID) and returns an `S3BatchDeleteResult` with per-key successes and failures. Requests are split at 1000 keys. |
| `listObjectVersionsPage(prefix:, pageSize:, keyMarker:, versionIdMarker:)` | Returns an `S3ListVersionsPage` of versions and delete markers, with markers for the next page. Requires provider versioning support. |
| `uploadBytes(bytes, objectKey, options:)` | Uploads in-memory bytes, up to `S3Client.hardMaxTransferBytes` (256 MiB). |
| `downloadBytes(objectKey, versionId:, maxBytes:)` | Downloads an object into memory as `Uint8List`, limited by `maxBytes` (default 16 MiB). |
| `download(objectKey, destinationPath, versionId:)` | Downloads an object to a local file; returns an empty string on success. |
| `deleteObject(objectKey, versionId:)` | Deletes an object, or a specific version; returns an empty string on success. |
| `isKeyBucketExist(objectKey)` | Returns `Future<bool>` indicating whether an object exists. |
| `getPresignedUrl(objectKey, expirationSeconds: 3600, versionId:)` | Returns a presigned GET URL as `Future<String>`, optionally for one version. Expiration must be between 1 and 604800 seconds. |
| `getPresignedPutUrl(objectKey, expirationSeconds: 3600, options: S3UploadOptions())` | Returns an `S3PresignedRequest` with the `url`, `method` (`PUT`), and the `headers` the uploader must send. |
| `close()` | Releases the native client. |

For example, after initializing `client`:

```dart
// Upload and list objects.
final key = await client.upload('/tmp/report.pdf', 'reports/report.pdf');
final objects = await client.listObjects();

// Check an object and create a one-hour download URL.
if (await client.isKeyBucketExist(key)) {
  final url = await client.getPresignedUrl(key, expirationSeconds: 3600);
  print(url);
}

// Download and delete an object.
await client.download(key, '/tmp/downloaded-report.pdf');
await client.deleteObject(key);
```

### Object information, paging, and uploads

```dart
// Inspect an object.
try {
  final meta = await client.headObject('reports/report.pdf');
  print('${meta.contentLength} bytes, ${meta.contentType}, ${meta.eTag}');
  print(meta.userMetadata);
} on S3NotFoundException {
  print('no such object');
} on S3AccessDeniedException {
  print('permission denied');
}

// Browse "folders" one page at a time.
String? token;
do {
  final page = await client.listObjectsPage(
    prefix: 'reports/',
    delimiter: '/',
    pageSize: 100,
    continuationToken: token,
  );
  page.commonPrefixes.forEach(print); // sub-folders
  page.objects.map((o) => '${o.key} ${o.size}').forEach(print);
  token = page.nextContinuationToken;
} while (token != null);

// Upload with a content type and user metadata.
await client.upload(
  '/tmp/report.pdf',
  'reports/report.pdf',
  options: const S3UploadOptions(
    contentType: 'application/pdf',
    metadata: {'owner': 'finance'},
  ),
);

// Let another party upload with a presigned PUT.
final put = await client.getPresignedPutUrl(
  'incoming/photo.jpg',
  expirationSeconds: 900,
  options: const S3UploadOptions(contentType: 'image/jpeg'),
);
final request = await HttpClient().putUrl(Uri.parse(put.url));
put.headers.forEach(request.headers.set);
request.add(await File('/tmp/photo.jpg').readAsBytes());
await (await request.close()).drain<void>();
```

Notes:

- User metadata keys must be non-empty and contain only HTTP token characters (letters, digits, and ``!#$%&'*+-.^_`|~``), unique ignoring case, with printable US-ASCII values; keys plus values may not exceed 2048 bytes. S3 returns metadata keys in lower case. Invalid options throw `ArgumentError`.
- The `host` header is omitted from `S3PresignedRequest.headers` because HTTP clients derive it from the URL. The AWS SDK does not sign `Content-Type`, so it is returned for the uploader to send but its value is not enforced by the signature; metadata headers are signed and must match.
- `ETag` values keep the quotes the service returns. `lastModified` is a UTC `DateTime`.

### Copies, batch deletes, versions, and byte transfers

```dart
// Copy within the bucket, replacing metadata.
final copy = await client.copyObject(
  'reports/old name.pdf',
  'reports/report.pdf',
  metadataBehavior: S3CopyMetadataBehavior.replace,
  options: const S3UploadOptions(contentType: 'application/pdf'),
);
print(copy.eTag);

// Delete a batch; failures are reported per key, not thrown.
final result = await client.deleteObjects([
  const S3ObjectIdentifier('tmp/a'),
  const S3ObjectIdentifier('tmp/b', versionId: 'v1'),
]);
for (final error in result.errors) {
  print('${error.key}: ${error.code} ${error.message}');
}

// Work with a specific version.
final old = await client.downloadBytes('reports/report.pdf', versionId: 'v1');
final page = await client.listObjectVersionsPage(prefix: 'reports/');
for (final version in page.versions) {
  print('${version.key} ${version.versionId} marker=${version.isDeleteMarker}');
}

// Small in-memory payloads.
await client.uploadBytes(
  Uint8List.fromList(utf8.encode('hello')),
  'notes/hello.txt',
  options: const S3UploadOptions(contentType: 'text/plain'),
);
```

Notes:

- `deleteObjects` splits requests at 1000 keys and never throws for individual key failures. If a whole chunk request fails, every key in that chunk is listed in `errors` and the remaining chunks still run. Check `result.errors`.
- `copyObject` percent-encodes unusual source keys. Single-request copies are limited by the provider (5 GB on AWS S3); larger copies throw `S3UnsupportedLargeCopyException`. Multipart copy is not implemented.
- Version listing requires provider support. Cloudflare R2 does not support object versioning, so `listObjectVersionsPage` returns the provider error there.
- `uploadBytes` and `downloadBytes` hold the whole payload in memory in both Dart and Go. They are limited to 256 MiB, and `downloadBytes` defaults to 16 MiB, throwing `S3SizeLimitException` above the limit. Use `upload` and `download` for large files; true streaming is a separate design.

S3 operation failures throw `S3Exception`, whose `message` contains the native error and whose `code` and `statusCode` carry the provider error code and HTTP status when available. Any operation can throw the subclasses `S3NotFoundException` (the object or bucket was not found) and `S3AccessDeniedException` (HTTP 401/403), so those cases can be told apart from other failures; both are still caught by `on S3Exception`. Calling an operation before initialization or after `close()` throws `StateError`. Invalid presigned URL expiration or page size throws `RangeError`; empty object keys and invalid options throw `ArgumentError`.

Operations return futures, but the underlying native calls run synchronously on the calling isolate. Use another isolate if a request must not block the calling isolate.

## Native artifacts

CI builds zipped shared libraries and SHA-256 checksums for tagged releases. These are provided for inspection and independent native integration; the Dart hook builds from source and does not download them. See [RELEASE_PROCESS.md](RELEASE_PROCESS.md). The raw native ABI is internal and may change.
