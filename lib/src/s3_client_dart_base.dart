import 'dart:typed_data';

import 's3_configuration.dart' show S3Configuration;
import 's3_exceptions.dart';
import 's3_ffi_bindings.dart';
import 's3_models.dart';

/// S3 client for interacting with AWS S3 using Go FFI
class S3Client {
  /// Default limit for [downloadBytes] and [uploadBytes] (16 MiB).
  static const int defaultMaxTransferBytes = 16 * 1024 * 1024;

  /// Hard cap for in-memory transfers (256 MiB).
  ///
  /// Larger objects must use [upload] or [download], which stream files.
  static const int hardMaxTransferBytes = 256 * 1024 * 1024;

  final S3FFIBindings _bindings;
  bool _initialized = false;

  /// Creates an S3 client. Call [close] when it is no longer needed.
  S3Client() : _bindings = S3FFIBindings();

  /// Releases the native client. A closed client can be initialized again.
  void close() {
    _bindings.close();
    _initialized = false;
  }

  /// Initialize the S3 client with bucket credentials, region, and endpoint
  ///
  /// [configuration] - S3Configuration object containing credentials, region, and endpoint
  void initialize({
    required S3Configuration configuration,
  }) {
    _bindings.initBucket(
      endpoint: configuration.endpoint,
      bucketName: configuration.bucketName,
      accessKeyId: configuration.accessKeyId,
      secretAccessKey: configuration.secretAccessKey,
      sessionToken: configuration.sessionToken,
      region: configuration.region,
      accountId: configuration.accountId,
    );
    _initialized = true;
  }

  /// Upload a file to S3
  ///
  /// [filePath] - Local path to the file to upload
  /// [objectKey] - The key (path) for the object in S3
  /// [options] - Optional content type and user metadata
  ///
  /// Returns the object key on success; throws [S3Exception] on failure and
  /// [ArgumentError] if [options] are invalid.
  Future<String> upload(
    String filePath,
    String objectKey, {
    S3UploadOptions options = const S3UploadOptions(),
  }) async {
    _ensureInitialized();
    _checkKey(objectKey);
    options.validate();
    return _bindings.upload(filePath, objectKey, options);
  }

  /// Fetch an object's metadata without downloading its body.
  ///
  /// Fields the service does not return are `null`. Throws
  /// [S3NotFoundException] when the object does not exist,
  /// [S3AccessDeniedException] when access is denied, and [S3Exception] for
  /// other failures. [ArgumentError] is thrown for an empty [objectKey].
  /// [versionId] fetches a specific version on versioned buckets.
  Future<S3ObjectMetadata> headObject(
    String objectKey, {
    String? versionId,
  }) async {
    _ensureInitialized();
    _checkKey(objectKey);
    return _bindings.head(objectKey, versionId);
  }

  /// List one page of objects.
  ///
  /// [prefix] limits results to keys starting with it. [delimiter] groups keys
  /// sharing a prefix up to the delimiter into [S3ListPage.commonPrefixes].
  /// [pageSize] must be between 1 and 1000 (the service default is used when
  /// omitted). Pass the previous page's [S3ListPage.nextContinuationToken] as
  /// [continuationToken] to continue; it must not be empty.
  ///
  /// Throws [RangeError] or [ArgumentError] for invalid inputs and
  /// [S3Exception] on failure.
  Future<S3ListPage> listObjectsPage({
    String? prefix,
    String? delimiter,
    int? pageSize,
    String? continuationToken,
  }) async {
    _ensureInitialized();
    if (pageSize != null && (pageSize < 1 || pageSize > 1000)) {
      throw RangeError.range(pageSize, 1, 1000, 'pageSize');
    }
    if (continuationToken != null && continuationToken.isEmpty) {
      throw ArgumentError.value(
        continuationToken,
        'continuationToken',
        'must not be empty',
      );
    }
    return _bindings.listPage(
      prefix: prefix,
      delimiter: delimiter,
      pageSize: pageSize,
      continuationToken: continuationToken,
    );
  }

  /// List all objects in the bucket
  ///
  /// Returns a list of object keys
  Future<List<String>> listObjects() async {
    _ensureInitialized();
    return _bindings.list();
  }

  /// Delete an object from S3
  ///
  /// [objectKey] - The key of the object to delete
  ///
  /// Returns an empty string on success; throws [S3Exception] on failure
  /// [versionId] deletes a specific version; without it, versioned buckets
  /// create a delete marker instead of removing data.
  Future<String> deleteObject(String objectKey, {String? versionId}) async {
    _ensureInitialized();
    _checkKey(objectKey);
    return _bindings.delete(objectKey, versionId);
  }

  /// Delete many objects in one call.
  ///
  /// Requests are split into chunks of 1000 keys, the S3 limit. Per-key
  /// failures are returned in [S3BatchDeleteResult.errors] instead of
  /// throwing, and a failed chunk request marks every key of that chunk as
  /// failed while the remaining chunks still run.
  Future<S3BatchDeleteResult> deleteObjects(
    List<S3ObjectIdentifier> objects,
  ) async {
    _ensureInitialized();
    for (final object in objects) {
      _checkKey(object.key);
    }
    if (objects.isEmpty) {
      return const S3BatchDeleteResult(deleted: [], errors: []);
    }
    return _bindings.deleteObjects(objects);
  }

  /// Copy an object to [destinationKey] in this client's bucket.
  ///
  /// [sourceBucket] defaults to this client's bucket; [sourceVersionId]
  /// selects a specific source version. [metadataBehavior] controls whether
  /// the source metadata is kept (the default) or replaced with [options].
  /// Passing [options] without [S3CopyMetadataBehavior.replace] throws
  /// [ArgumentError].
  ///
  /// Source keys are percent-encoded for the copy-source header. Copies
  /// larger than the provider's single-request limit (5 GB on AWS S3) throw
  /// [S3UnsupportedLargeCopyException]; multipart copy is not implemented.
  Future<S3CopyResult> copyObject(
    String sourceKey,
    String destinationKey, {
    String? sourceBucket,
    String? sourceVersionId,
    S3CopyMetadataBehavior metadataBehavior = S3CopyMetadataBehavior.copy,
    S3UploadOptions options = const S3UploadOptions(),
  }) async {
    _ensureInitialized();
    _checkKey(sourceKey);
    _checkKey(destinationKey);
    final replace = metadataBehavior == S3CopyMetadataBehavior.replace;
    if (!replace &&
        (options.contentType != null || options.metadata.isNotEmpty)) {
      throw ArgumentError.value(
        metadataBehavior,
        'metadataBehavior',
        'options are only accepted with S3CopyMetadataBehavior.replace',
      );
    }
    options.validate();
    return _bindings.copyObject(
      sourceBucket: sourceBucket ?? '',
      sourceKey: sourceKey,
      destinationKey: destinationKey,
      sourceVersionId: sourceVersionId,
      replaceMetadata: replace,
      options: options,
    );
  }

  /// List one page of object versions and delete markers.
  ///
  /// [pageSize] must be between 1 and 1000. Pass the previous page's
  /// [S3ListVersionsPage.nextKeyMarker] and `nextVersionIdMarker` to continue.
  /// Providers without versioning support (for example Cloudflare R2) return
  /// their error as an [S3Exception].
  Future<S3ListVersionsPage> listObjectVersionsPage({
    String? prefix,
    int? pageSize,
    String? keyMarker,
    String? versionIdMarker,
  }) async {
    _ensureInitialized();
    if (pageSize != null && (pageSize < 1 || pageSize > 1000)) {
      throw RangeError.range(pageSize, 1, 1000, 'pageSize');
    }
    if (keyMarker != null && keyMarker.isEmpty) {
      throw ArgumentError.value(keyMarker, 'keyMarker', 'must not be empty');
    }
    return _bindings.listVersions(
      prefix: prefix,
      pageSize: pageSize,
      keyMarker: keyMarker,
      versionIdMarker: versionIdMarker,
    );
  }

  /// Download an object from S3 to a local file
  ///
  /// [objectKey] - The key of the object to download
  /// [destinationPath] - Local path where the file will be saved
  ///
  /// Returns an empty string on success; throws [S3Exception] on failure
  Future<String> download(
    String objectKey,
    String destinationPath, {
    String? versionId,
  }) async {
    _ensureInitialized();
    _checkKey(objectKey);
    return _bindings.download(objectKey, destinationPath, versionId);
  }

  /// Upload in-memory bytes to S3.
  ///
  /// The payload must not exceed [hardMaxTransferBytes]; larger payloads must
  /// use [upload] so the file is streamed. Throws [S3SizeLimitException] when
  /// the limit is exceeded and [ArgumentError] for invalid [options].
  Future<String> uploadBytes(
    Uint8List bytes,
    String objectKey, {
    S3UploadOptions options = const S3UploadOptions(),
  }) async {
    _ensureInitialized();
    _checkKey(objectKey);
    options.validate();
    if (bytes.length > hardMaxTransferBytes) {
      throw S3SizeLimitException(
        'upload of ${bytes.length} bytes exceeds the '
        '$hardMaxTransferBytes byte limit',
      );
    }
    return _bindings.uploadBytes(objectKey, bytes, options);
  }

  /// Download an object into memory.
  ///
  /// [maxBytes] defaults to [defaultMaxTransferBytes] and must be between 1
  /// and [hardMaxTransferBytes]. Objects larger than the limit throw
  /// [S3SizeLimitException] before the body is buffered; use [download] for
  /// large objects. [versionId] downloads a specific version.
  Future<Uint8List> downloadBytes(
    String objectKey, {
    String? versionId,
    int maxBytes = defaultMaxTransferBytes,
  }) async {
    _ensureInitialized();
    _checkKey(objectKey);
    if (maxBytes < 1 || maxBytes > hardMaxTransferBytes) {
      throw RangeError.range(maxBytes, 1, hardMaxTransferBytes, 'maxBytes');
    }
    return _bindings.downloadBytes(objectKey, versionId, maxBytes);
  }

  /// Get a presigned URL for an object
  ///
  /// [objectKey] - The key of the object
  /// [expirationSeconds] - How long the URL should be valid (in seconds)
  ///
  /// Returns the presigned URL; throws [S3Exception] on failure
  Future<String> getPresignedUrl(
    String objectKey, {
    int expirationSeconds = 3600,
    String? versionId,
  }) async {
    _ensureInitialized();
    _checkKey(objectKey);
    return _bindings.getPresignedUrl(objectKey, expirationSeconds, versionId);
  }

  /// Get a presigned PUT request for uploading an object
  ///
  /// [objectKey] - The key of the object
  /// [expirationSeconds] - How long the URL should be valid (1 to 604800)
  /// [options] - Optional content type and user metadata
  ///
  /// The caller must send an HTTP PUT to the URL with every returned header.
  /// Metadata headers are covered by the signature; `content-type` is not
  /// signed by the SDK, so it is returned to be sent but not enforced.
  /// Throws [RangeError] for an invalid expiration and [ArgumentError] for an
  /// empty key or invalid options.
  Future<S3PresignedRequest> getPresignedPutUrl(
    String objectKey, {
    int expirationSeconds = 3600,
    S3UploadOptions options = const S3UploadOptions(),
  }) async {
    _ensureInitialized();
    _checkKey(objectKey);
    if (expirationSeconds <= 0 || expirationSeconds > 604800) {
      throw RangeError.range(expirationSeconds, 1, 604800, 'expirationSeconds');
    }
    options.validate();
    return _bindings.presignPut(objectKey, expirationSeconds, options);
  }

  /// Check if an object exists in the bucket
  ///
  /// [objectKey] - The key of the object to check
  ///
  /// Returns true if the object exists, false if it does not, and throws
  /// [S3Exception] for other failures
  Future<bool> isKeyBucketExist(String objectKey) async {
    _ensureInitialized();
    _checkKey(objectKey);
    return _bindings.isKeyBucketExist(objectKey);
  }

  void _checkKey(String key) {
    if (key.isEmpty) {
      throw ArgumentError.value(key, 'objectKey', 'must not be empty');
    }
  }

  void _ensureInitialized() {
    if (!_initialized) {
      throw StateError('S3Client not initialized. Call initialize() first.');
    }
  }
}
