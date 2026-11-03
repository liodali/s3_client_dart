import 'dart:convert';
import 'dart:ffi';

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
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>)>(symbol: 's3Head')
external Pointer<Utf8> _head(int id, Pointer<Utf8> key);
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
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>)>(symbol: 's3Delete')
external Pointer<Utf8> _delete(int id, Pointer<Utf8> key);
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>, Pointer<Utf8>)>(
  symbol: 's3Download',
)
external Pointer<Utf8> _download(int id, Pointer<Utf8> key, Pointer<Utf8> path);
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>, Int32)>(
  symbol: 's3Presign',
)
external Pointer<Utf8> _presign(int id, Pointer<Utf8> key, int seconds);
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

  S3ObjectMetadata head(String key) => using(
    (a) => S3ObjectMetadata.fromJson(
      _decode(_head(_client, key.toNativeUtf8(allocator: a)))
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
  String delete(String key) => using(
    (a) => _decode(_delete(_client, key.toNativeUtf8(allocator: a))) as String,
  );
  String download(String key, String path) => using(
    (a) =>
        _decode(
              _download(
                _client,
                key.toNativeUtf8(allocator: a),
                path.toNativeUtf8(allocator: a),
              ),
            )
            as String,
  );
  String getPresignedUrl(String key, int seconds) {
    if (seconds <= 0 || seconds > 604800) {
      throw RangeError.range(seconds, 1, 604800, 'expirationSeconds');
    }
    return using(
      (a) =>
          _decode(_presign(_client, key.toNativeUtf8(allocator: a), seconds))
              as String,
    );
  }

  bool isKeyBucketExist(String key) => using(
    (a) => _decode(_exists(_client, key.toNativeUtf8(allocator: a))) as bool,
  );
}
