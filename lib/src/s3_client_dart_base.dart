import 'package:s3_client_dart/src/s3_configuration.dart' show S3Configuration;

import 's3_ffi_bindings.dart';

/// S3 client for interacting with AWS S3 using Go FFI
class S3Client {
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
  ///
  /// Returns the object key on success; throws [S3Exception] on failure
  Future<String> upload(String filePath, String objectKey) async {
    _ensureInitialized();
    return _bindings.upload(filePath, objectKey);
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
  Future<String> deleteObject(String objectKey) async {
    _ensureInitialized();
    return _bindings.delete(objectKey);
  }

  /// Download an object from S3 to a local file
  ///
  /// [objectKey] - The key of the object to download
  /// [destinationPath] - Local path where the file will be saved
  ///
  /// Returns an empty string on success; throws [S3Exception] on failure
  Future<String> download(String objectKey, String destinationPath) async {
    _ensureInitialized();
    return _bindings.download(objectKey, destinationPath);
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
  }) async {
    _ensureInitialized();
    return _bindings.getPresignedUrl(objectKey, expirationSeconds);
  }

  /// Check if an object exists in the bucket
  ///
  /// [objectKey] - The key of the object to check
  ///
  /// Returns true if the object exists, false if it does not, and throws
  /// [S3Exception] for other failures
  Future<bool> isKeyBucketExist(String objectKey) async {
    _ensureInitialized();
    return _bindings.isKeyBucketExist(objectKey);
  }

  void _ensureInitialized() {
    if (!_initialized) {
      throw StateError('S3Client not initialized. Call initialize() first.');
    }
  }
}

/// Exception thrown when S3 operations fail
class S3Exception implements Exception {
  final String message;

  S3Exception(this.message);

  @override
  String toString() => 'S3Exception: $message';
}
