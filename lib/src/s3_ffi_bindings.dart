import 'dart:convert';
import 'dart:ffi';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';

import 's3_exceptions.dart';
import 's3_models.dart';

@Native<
  Pointer<Utf8> Function(
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
  )
>(symbol: 's3Create')
external Pointer<Utf8> _create(
  Pointer<Utf8> endpoint,
  Pointer<Utf8> bucket,
  Pointer<Utf8> key,
  Pointer<Utf8> secret,
  Pointer<Utf8> token,
  Pointer<Utf8> region,
  Pointer<Utf8> account,
);
@Native<Void Function(Uint64)>(symbol: 's3Close')
external void _close(int id);
@Native<Void Function(Pointer<Utf8>)>(symbol: 's3Free')
external void _free(Pointer<Utf8> value);
@Native<
  Pointer<Utf8> Function(
    Uint64,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
  )
>(symbol: 's3Upload')
external Pointer<Utf8> _upload(
  int id,
  Pointer<Utf8> path,
  Pointer<Utf8> key,
  Pointer<Utf8> contentType,
  Pointer<Utf8> metadata,
);
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>, Pointer<Utf8>)>(
  symbol: 's3Head',
)
external Pointer<Utf8> _head(
  int id,
  Pointer<Utf8> key,
  Pointer<Utf8> versionId,
);
@Native<
  Pointer<Utf8> Function(
    Uint64,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Int32,
    Pointer<Utf8>,
  )
>(symbol: 's3ListPage')
external Pointer<Utf8> _listPage(
  int id,
  Pointer<Utf8> prefix,
  Pointer<Utf8> delimiter,
  int pageSize,
  Pointer<Utf8> token,
);
@Native<
  Pointer<Utf8> Function(
    Uint64,
    Pointer<Utf8>,
    Int32,
    Pointer<Utf8>,
    Pointer<Utf8>,
  )
>(symbol: 's3PresignPut')
external Pointer<Utf8> _presignPut(
  int id,
  Pointer<Utf8> key,
  int seconds,
  Pointer<Utf8> contentType,
  Pointer<Utf8> metadata,
);
@Native<Pointer<Utf8> Function(Uint64)>(symbol: 's3List')
external Pointer<Utf8> _list(int id);
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>, Pointer<Utf8>)>(
  symbol: 's3Delete',
)
external Pointer<Utf8> _delete(
  int id,
  Pointer<Utf8> key,
  Pointer<Utf8> versionId,
);
@Native<
  Pointer<Utf8> Function(Uint64, Pointer<Utf8>, Pointer<Utf8>, Pointer<Utf8>)
>(symbol: 's3Download')
external Pointer<Utf8> _download(
  int id,
  Pointer<Utf8> key,
  Pointer<Utf8> path,
  Pointer<Utf8> versionId,
);
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>, Int32, Pointer<Utf8>)>(
  symbol: 's3Presign',
)
external Pointer<Utf8> _presign(
  int id,
  Pointer<Utf8> key,
  int seconds,
  Pointer<Utf8> versionId,
);
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>)>(
  symbol: 's3DeleteObjects',
)
external Pointer<Utf8> _deleteObjects(int id, Pointer<Utf8> payload);
@Native<
  Pointer<Utf8> Function(
    Uint64,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Int32,
  )
>(symbol: 's3CopyObject')
external Pointer<Utf8> _copyObject(
  int id,
  Pointer<Utf8> sourceBucket,
  Pointer<Utf8> sourceKey,
  Pointer<Utf8> destinationKey,
  Pointer<Utf8> sourceVersionId,
  Pointer<Utf8> contentType,
  Pointer<Utf8> metadata,
  int replaceMetadata,
);
@Native<
  Pointer<Utf8> Function(
    Uint64,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Int32,
  )
>(symbol: 's3ListVersions')
external Pointer<Utf8> _listVersions(
  int id,
  Pointer<Utf8> prefix,
  Pointer<Utf8> keyMarker,
  Pointer<Utf8> versionIdMarker,
  int pageSize,
);
@Native<
  Pointer<Utf8> Function(
    Uint64,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Pointer<Uint8>,
    Int64,
  )
>(symbol: 's3UploadBytes')
external Pointer<Utf8> _uploadBytes(
  int id,
  Pointer<Utf8> key,
  Pointer<Utf8> contentType,
  Pointer<Utf8> metadata,
  Pointer<Uint8> data,
  int length,
);
@Native<
  Pointer<Utf8> Function(
    Uint64,
    Pointer<Utf8>,
    Pointer<Utf8>,
    Int64,
    Pointer<Int64>,
  )
>(symbol: 's3DownloadBytes')
external Pointer<Utf8> _downloadBytes(
  int id,
  Pointer<Utf8> key,
  Pointer<Utf8> versionId,
  int maxBytes,
  Pointer<Int64> outLength,
);
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>)>(symbol: 's3Exists')
external Pointer<Utf8> _exists(int id, Pointer<Utf8> key);

class S3FFIBindings {
  int? _id;

  int get _client =>
      _id ?? (throw StateError('S3 client is not initialized or is closed.'));

  dynamic _decode(Pointer<Utf8> pointer) {
    if (pointer == nullptr) {
      throw S3Exception('Native library returned a null response');
    }
    try {
      final response =
          jsonDecode(pointer.toDartString()) as Map<String, dynamic>;
      if (response['error'] case final String error) {
        final code = response['code'] as String?;
        final status = response['status'] as int?;
        throw switch (response['kind']) {
          'not_found' => S3NotFoundException(
            error,
            code: code,
            statusCode: status,
          ),
          'access_denied' => S3AccessDeniedException(
            error,
            code: code,
            statusCode: status,
          ),
          'size_limit' => S3SizeLimitException(
            error,
            code: code,
            statusCode: status,
          ),
          'unsupported_large_copy' => S3UnsupportedLargeCopyException(
            error,
            code: code,
            statusCode: status,
          ),
          _ => S3Exception(error, code: code, statusCode: status),
        };
      }
      return response['value'];
    } finally {
      _free(pointer);
    }
  }

  void initBucket({
    required String endpoint,
    required String bucketName,
    required String accessKeyId,
    required String secretAccessKey,
    required String sessionToken,
    required String region,
    required String accountId,
  }) {
    final id = using(
      (arena) =>
          _decode(
                _create(
                  endpoint.toNativeUtf8(allocator: arena),
                  bucketName.toNativeUtf8(allocator: arena),
                  accessKeyId.toNativeUtf8(allocator: arena),
                  secretAccessKey.toNativeUtf8(allocator: arena),
                  sessionToken.toNativeUtf8(allocator: arena),
                  region.toNativeUtf8(allocator: arena),
                  accountId.toNativeUtf8(allocator: arena),
                ),
              )
              as int,
    );
    final previous = _id;
    _id = id;
    if (previous != null) _close(previous);
  }

  void close() {
    final id = _id;
    _id = null;
    if (id != null) _close(id);
  }

  String upload(String path, String key, S3UploadOptions options) => using(
    (a) =>
        _decode(
              _upload(
                _client,
                path.toNativeUtf8(allocator: a),
                key.toNativeUtf8(allocator: a),
                (options.contentType ?? '').toNativeUtf8(allocator: a),
                _metadataJson(options).toNativeUtf8(allocator: a),
              ),
            )
            as String,
  );

  static String _metadataJson(S3UploadOptions options) =>
      options.metadata.isEmpty ? '' : jsonEncode(options.metadata);

  S3ObjectMetadata head(String key, String? versionId) => using(
    (a) => S3ObjectMetadata.fromJson(
      _decode(
            _head(
              _client,
              key.toNativeUtf8(allocator: a),
              (versionId ?? '').toNativeUtf8(allocator: a),
            ),
          )
          as Map<String, dynamic>,
    ),
  );

  S3ListPage listPage({
    String? prefix,
    String? delimiter,
    int? pageSize,
    String? continuationToken,
  }) => using(
    (a) => S3ListPage.fromJson(
      _decode(
            _listPage(
              _client,
              (prefix ?? '').toNativeUtf8(allocator: a),
              (delimiter ?? '').toNativeUtf8(allocator: a),
              pageSize ?? 0,
              (continuationToken ?? '').toNativeUtf8(allocator: a),
            ),
          )
          as Map<String, dynamic>,
    ),
  );

  S3PresignedRequest presignPut(
    String key,
    int seconds,
    S3UploadOptions options,
  ) => using(
    (a) => S3PresignedRequest.fromJson(
      _decode(
            _presignPut(
              _client,
              key.toNativeUtf8(allocator: a),
              seconds,
              (options.contentType ?? '').toNativeUtf8(allocator: a),
              _metadataJson(options).toNativeUtf8(allocator: a),
            ),
          )
          as Map<String, dynamic>,
    ),
  );

  List<String> list() =>
      (_decode(_list(_client)) as List<dynamic>).cast<String>();
  String delete(String key, String? versionId) => using(
    (a) =>
        _decode(
              _delete(
                _client,
                key.toNativeUtf8(allocator: a),
                (versionId ?? '').toNativeUtf8(allocator: a),
              ),
            )
            as String,
  );
  String download(String key, String path, String? versionId) => using(
    (a) =>
        _decode(
              _download(
                _client,
                key.toNativeUtf8(allocator: a),
                path.toNativeUtf8(allocator: a),
                (versionId ?? '').toNativeUtf8(allocator: a),
              ),
            )
            as String,
  );
  String getPresignedUrl(String key, int seconds, String? versionId) {
    if (seconds <= 0 || seconds > 604800) {
      throw RangeError.range(seconds, 1, 604800, 'expirationSeconds');
    }
    return using(
      (a) =>
          _decode(
                _presign(
                  _client,
                  key.toNativeUtf8(allocator: a),
                  seconds,
                  (versionId ?? '').toNativeUtf8(allocator: a),
                ),
              )
              as String,
    );
  }

  S3BatchDeleteResult deleteObjects(List<S3ObjectIdentifier> objects) => using(
    (a) => S3BatchDeleteResult.fromJson(
      _decode(
            _deleteObjects(
              _client,
              jsonEncode({
                'items': objects.map((o) => o.toJson()).toList(),
              }).toNativeUtf8(allocator: a),
            ),
          )
          as Map<String, dynamic>,
    ),
  );

  S3CopyResult copyObject({
    required String sourceBucket,
    required String sourceKey,
    required String destinationKey,
    required String? sourceVersionId,
    required bool replaceMetadata,
    required S3UploadOptions options,
  }) => using(
    (a) => S3CopyResult.fromJson(
      _decode(
            _copyObject(
              _client,
              sourceBucket.toNativeUtf8(allocator: a),
              sourceKey.toNativeUtf8(allocator: a),
              destinationKey.toNativeUtf8(allocator: a),
              (sourceVersionId ?? '').toNativeUtf8(allocator: a),
              (options.contentType ?? '').toNativeUtf8(allocator: a),
              _metadataJson(options).toNativeUtf8(allocator: a),
              replaceMetadata ? 1 : 0,
            ),
          )
          as Map<String, dynamic>,
    ),
  );

  S3ListVersionsPage listVersions({
    String? prefix,
    int? pageSize,
    String? keyMarker,
    String? versionIdMarker,
  }) => using(
    (a) => S3ListVersionsPage.fromJson(
      _decode(
            _listVersions(
              _client,
              (prefix ?? '').toNativeUtf8(allocator: a),
              (keyMarker ?? '').toNativeUtf8(allocator: a),
              (versionIdMarker ?? '').toNativeUtf8(allocator: a),
              pageSize ?? 0,
            ),
          )
          as Map<String, dynamic>,
    ),
  );

  String uploadBytes(String key, Uint8List data, S3UploadOptions options) =>
      using((a) {
        final buffer = a<Uint8>(data.isEmpty ? 1 : data.length);
        buffer.asTypedList(data.length).setAll(0, data);
        return _decode(
              _uploadBytes(
                _client,
                key.toNativeUtf8(allocator: a),
                (options.contentType ?? '').toNativeUtf8(allocator: a),
                _metadataJson(options).toNativeUtf8(allocator: a),
                buffer,
                data.length,
              ),
            )
            as String;
      });

  Uint8List downloadBytes(String key, String? versionId, int maxBytes) =>
      using((a) {
        final outLength = a<Int64>();
        final pointer = _downloadBytes(
          _client,
          key.toNativeUtf8(allocator: a),
          (versionId ?? '').toNativeUtf8(allocator: a),
          maxBytes,
          outLength,
        );
        if (outLength.value == -1) {
          // A JSON error envelope; _decode frees the pointer and throws.
          _decode(pointer);
        }
        try {
          return Uint8List.fromList(
            pointer.cast<Uint8>().asTypedList(outLength.value),
          );
        } finally {
          _free(pointer);
        }
      });

  bool isKeyBucketExist(String key) => using(
    (a) => _decode(_exists(_client, key.toNativeUtf8(allocator: a))) as bool,
  );
}
