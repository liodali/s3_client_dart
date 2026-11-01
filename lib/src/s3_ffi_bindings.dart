import 'dart:convert';
import 'dart:ffi';

import 'package:ffi/ffi.dart';

import 's3_client_dart_base.dart' show S3Exception;

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
@Native<Pointer<Utf8> Function(Uint64, Pointer<Utf8>, Pointer<Utf8>)>(
  symbol: 's3Upload',
)
external Pointer<Utf8> _upload(int id, Pointer<Utf8> path, Pointer<Utf8> key);
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
      if (response['error'] case final String error) throw S3Exception(error);
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

  String upload(String path, String key) => using(
    (a) =>
        _decode(
              _upload(
                _client,
                path.toNativeUtf8(allocator: a),
                key.toNativeUtf8(allocator: a),
              ),
            )
            as String,
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
