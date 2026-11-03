final _tokenKey = RegExp(r"^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$");

/// Options applied when uploading an object or creating a presigned PUT.
class S3UploadOptions {
  /// Value stored as the object's `Content-Type`, or `null` to use the
  /// service default.
  final String? contentType;

  /// User metadata stored with the object (sent as `x-amz-meta-*` headers).
  ///
  /// Keys must be non-empty and contain only HTTP token characters (letters,
  /// digits, and ``!#$%&'*+-.^_`|~``), and must be unique ignoring case (S3 lower-cases them). Values
  /// must be printable US-ASCII. Keys plus values are limited to 2048 bytes.
  final Map<String, String> metadata;

  const S3UploadOptions({this.contentType, this.metadata = const {}});

  /// Throws [ArgumentError] if the options cannot be sent to S3.
  void validate() {
    final type = contentType;
    if (type != null &&
        (type.isEmpty || type.codeUnits.any((c) => c < 0x20 || c == 0x7f))) {
      throw ArgumentError.value(
        type,
        'contentType',
        'must be non-empty and contain no control characters',
      );
    }
    var size = 0;
    final seen = <String>{};
    metadata.forEach((key, value) {
      if (key.isEmpty || !_tokenKey.hasMatch(key)) {
        throw ArgumentError.value(
          key,
          'metadata',
          'metadata keys may only contain HTTP token characters',
        );
      }
      if (!seen.add(key.toLowerCase())) {
        throw ArgumentError.value(key, 'metadata', 'duplicate metadata key');
      }
      if (value.codeUnits.any((c) => c < 0x20 || c >= 0x7f)) {
        throw ArgumentError.value(
          value,
          'metadata',
          'metadata value for "$key" must be printable US-ASCII',
        );
      }
      size += key.length + value.length;
    });
    if (size > 2048) {
      throw ArgumentError.value(
        size,
        'metadata',
        'metadata exceeds 2048 bytes',
      );
    }
  }
}

DateTime? _time(Object? value) =>
    value is String ? DateTime.parse(value) : null;

/// Metadata returned by `S3Client.headObject`.
///
/// Values the service did not return are `null` (or empty for [userMetadata]).
class S3ObjectMetadata {
  final int? contentLength;
  final String? contentType;

  /// Entity tag as returned by the service, including quotes.
  final String? eTag;
  final DateTime? lastModified;

  /// User metadata without the `x-amz-meta-` prefix. Keys are lower-case.
  final Map<String, String> userMetadata;
  final String? versionId;

  const S3ObjectMetadata({
    this.contentLength,
    this.contentType,
    this.eTag,
    this.lastModified,
    this.userMetadata = const {},
    this.versionId,
  });

  factory S3ObjectMetadata.fromJson(Map<String, dynamic> json) =>
      S3ObjectMetadata(
        contentLength: json['contentLength'] as int?,
        contentType: json['contentType'] as String?,
        eTag: json['eTag'] as String?,
        lastModified: _time(json['lastModified']),
        userMetadata: Map.unmodifiable(
          ((json['metadata'] as Map?) ?? const {}).cast<String, String>(),
        ),
        versionId: json['versionId'] as String?,
      );
}

/// An object entry in an [S3ListPage].
class S3ObjectSummary {
  final String key;
  final int? size;
  final String? eTag;
  final DateTime? lastModified;
  final String? storageClass;

  const S3ObjectSummary({
    required this.key,
    this.size,
    this.eTag,
    this.lastModified,
    this.storageClass,
  });

  factory S3ObjectSummary.fromJson(Map<String, dynamic> json) =>
      S3ObjectSummary(
        key: json['key'] as String,
        size: json['size'] as int?,
        eTag: json['eTag'] as String?,
        lastModified: _time(json['lastModified']),
        storageClass: json['storageClass'] as String?,
      );
}

/// One page of a listing returned by `S3Client.listObjectsPage`.
class S3ListPage {
  final List<S3ObjectSummary> objects;

  /// Key prefixes grouped by the request delimiter.
  final List<String> commonPrefixes;

  /// Pass to the next call to continue the listing; `null` on the last page.
  final String? nextContinuationToken;

  const S3ListPage({
    required this.objects,
    required this.commonPrefixes,
    this.nextContinuationToken,
  });

  bool get hasMore => nextContinuationToken != null;

  factory S3ListPage.fromJson(Map<String, dynamic> json) => S3ListPage(
    objects: List.unmodifiable(
      ((json['objects'] as List?) ?? const []).map(
        (o) => S3ObjectSummary.fromJson(o as Map<String, dynamic>),
      ),
    ),
    commonPrefixes: List.unmodifiable(
      ((json['commonPrefixes'] as List?) ?? const []).cast<String>(),
    ),
    nextContinuationToken: json['nextContinuationToken'] as String?,
  );
}

/// A presigned request: send [method] to [url] with every entry of [headers].
class S3PresignedRequest {
  final String url;
  final String method;

  /// Header names are lower-case. The `host` header is omitted because HTTP
  /// clients derive it from [url].
  final Map<String, String> headers;

  const S3PresignedRequest({
    required this.url,
    required this.method,
    required this.headers,
  });

  factory S3PresignedRequest.fromJson(Map<String, dynamic> json) =>
      S3PresignedRequest(
        url: json['url'] as String,
        method: json['method'] as String,
        headers: Map.unmodifiable(
          ((json['headers'] as Map?) ?? const {}).cast<String, String>(),
        ),
      );
}
