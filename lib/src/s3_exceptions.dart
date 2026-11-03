/// Exception thrown when S3 operations fail.
class S3Exception implements Exception {
  /// Error text reported by the native library.
  final String message;

  /// Provider error code (for example `AccessDenied`), when reported.
  final String? code;

  /// HTTP status code of the failed response, when available.
  final int? statusCode;

  S3Exception(this.message, {this.code, this.statusCode});

  @override
  String toString() => 'S3Exception: $message';
}

/// The requested object does not exist.
class S3NotFoundException extends S3Exception {
  S3NotFoundException(super.message, {super.code, super.statusCode});

  @override
  String toString() => 'S3NotFoundException: $message';
}

/// The request was rejected because of missing or insufficient permissions.
class S3AccessDeniedException extends S3Exception {
  S3AccessDeniedException(super.message, {super.code, super.statusCode});

  @override
  String toString() => 'S3AccessDeniedException: $message';
}
