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
| `headObject(objectKey)` | Returns `S3ObjectMetadata` (content length, content type, ETag, last-modified time, user metadata, version ID) without downloading the body. Values the service omits are `null`. |
| `download(objectKey, destinationPath)` | Downloads an object to a local file; returns an empty string on success. |
| `deleteObject(objectKey)` | Deletes an object; returns an empty string on success. |
| `isKeyBucketExist(objectKey)` | Returns `Future<bool>` indicating whether an object exists. |
| `getPresignedUrl(objectKey, expirationSeconds: 3600)` | Returns a presigned GET URL as `Future<String>`. Expiration must be between 1 and 604800 seconds. |
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

- User metadata keys must be non-empty US-ASCII without whitespace or `:`, unique ignoring case, with printable US-ASCII values; keys plus values may not exceed 2048 bytes. S3 returns metadata keys in lower case. Invalid options throw `ArgumentError`.
- The `host` header is omitted from `S3PresignedRequest.headers` because HTTP clients derive it from the URL. The AWS SDK does not sign `Content-Type`, so it is returned for the uploader to send but its value is not enforced by the signature; metadata headers are signed and must match.
- `ETag` values keep the quotes the service returns. `lastModified` is a UTC `DateTime`.

S3 operation failures throw `S3Exception`, whose `message` contains the native error and whose `code` and `statusCode` carry the provider error code and HTTP status when available. `headObject` throws the subclasses `S3NotFoundException` and `S3AccessDeniedException` so those cases can be told apart from other failures. Calling an operation before initialization or after `close()` throws `StateError`. Invalid presigned URL expiration or page size throws `RangeError`; empty keys and invalid options throw `ArgumentError`.

Operations return futures, but the underlying native calls run synchronously on the calling isolate. Use another isolate if a request must not block the calling isolate.

## Native artifacts

CI builds zipped shared libraries and SHA-256 checksums for tagged releases. These are provided for inspection and independent native integration; the Dart hook builds from source and does not download them. See [RELEASE_PROCESS.md](RELEASE_PROCESS.md). The raw native ABI is internal and may change.
