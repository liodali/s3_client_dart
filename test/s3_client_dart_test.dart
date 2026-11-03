import 'dart:convert';
import 'dart:io';
import 'dart:isolate';

import 'package:s3_client_dart/s3_client_dart.dart';
import 'package:test/test.dart';

void _serve(SendPort ready) async {
  final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
  ready.send(server.port);
  var lastPut = <String, String>{};
  await for (final request in server) {
    await request.drain<void>();
    if (request.uri.path == '/__last_put') {
      request.response.write(jsonEncode(lastPut));
    } else if (request.method == 'PUT') {
      lastPut = {};
      request.headers.forEach((name, values) => lastPut[name] = values.join());
      request.response.statusCode = 200;
    } else if (request.method == 'GET' &&
        request.uri.queryParameters.containsKey('prefix')) {
      request.response.headers.contentType = ContentType('application', 'xml');
      final q = request.uri.queryParameters;
      const header =
          '<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">';
      if (q['prefix'] == 'empty/') {
        request.response.write(
          '$header<IsTruncated>false</IsTruncated></ListBucketResult>',
        );
      } else if (q['delimiter'] != null) {
        request.response.write(
          '$header<IsTruncated>false</IsTruncated><Contents><Key>${q['prefix']}top.txt</Key><Size>7</Size><ETag>"e"</ETag><LastModified>2020-01-02T03:04:05.000Z</LastModified><StorageClass>STANDARD</StorageClass></Contents><CommonPrefixes><Prefix>${q['prefix']}sub/</Prefix></CommonPrefixes></ListBucketResult>',
        );
      } else {
        final token = q['continuation-token'];
        final next = token == null ? 'p2' : (token == 'p2' ? 'p3' : null);
        final index = token == null ? 1 : (token == 'p2' ? 2 : 3);
        request.response.write(
          '$header<IsTruncated>${next != null}</IsTruncated>${next == null ? '' : '<NextContinuationToken>$next</NextContinuationToken>'}<Contents><Key>${q['prefix']}$index-of-${q['max-keys']}</Key></Contents></ListBucketResult>',
        );
      }
    } else if (request.method == 'GET' &&
        request.uri.queryParameters.containsKey('list-type')) {
      final bucket = request.uri.pathSegments.first;
      if (bucket == 'failure') {
        request.response.statusCode = 500;
        request.response.write(
          '<Error><Code>InternalError</Code><Message>failure</Message></Error>',
        );
      } else {
        final second = request.uri.queryParameters.containsKey(
          'continuation-token',
        );
        request.response.headers.contentType = ContentType(
          'application',
          'xml',
        );
        request.response.write(
          '<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>$bucket</Name><IsTruncated>${!second}</IsTruncated>${second ? '' : '<NextContinuationToken>next</NextContinuationToken>'}<Contents><Key>$bucket-${second ? 'two' : 'one'}</Key></Contents></ListBucketResult>',
        );
      }
    } else if (request.method == 'HEAD') {
      switch (request.uri.pathSegments.last) {
        case 'full':
          request.response.headers
            ..set('content-type', 'text/plain')
            ..set('etag', '"abc"')
            ..set('last-modified', 'Mon, 02 Jan 2006 15:04:05 GMT')
            ..set('x-amz-meta-owner', 'me')
            ..set('x-amz-version-id', 'v1');
          request.response.contentLength = 5;
        case 'sparse':
          break;
        case 'denied':
          request.response.statusCode = 403;
        case 'broken':
          request.response.statusCode = 400;
        default:
          request.response.statusCode = 404;
          request.response.headers.set('x-amz-error-code', 'NotFound');
      }
    } else {
      request.response.statusCode = 200;
    }
    await request.response.close();
  }
}

S3Configuration _config(int port, String bucket) => S3Configuration(
  endpoint: 'http://127.0.0.1:$port',
  bucketName: bucket,
  accessKeyId: 'test-key',
  secretAccessKey: 'test-secret',
  sessionToken: '',
  accountId: '',
  region: 'us-east-1',
);

void main() {
  late Isolate server;
  late int port;
  setUpAll(() async {
    final ready = ReceivePort();
    server = await Isolate.spawn(_serve, ready.sendPort);
    port = await ready.first as int;
    ready.close();
  });
  tearDownAll(() => server.kill(priority: Isolate.immediate));

  test('clients retain their own bucket and list all pages', () async {
    final first = S3Client()..initialize(configuration: _config(port, 'first'));
    final second = S3Client()
      ..initialize(configuration: _config(port, 'second'));
    addTearDown(first.close);
    addTearDown(second.close);
    expect(await first.listObjects(), ['first-one', 'first-two']);
    expect(await second.listObjects(), ['second-one', 'second-two']);
    expect(await first.isKeyBucketExist('missing'), isFalse);
    expect(await first.getPresignedUrl('key'), contains('/first/key'));
  });

  test('native failures are returned as S3Exception', () async {
    final client = S3Client()
      ..initialize(configuration: _config(port, 'failure'));
    addTearDown(client.close);
    await expectLater(client.listObjects(), throwsA(isA<S3Exception>()));
  });

  test('close and expiration validation', () async {
    final client = S3Client()
      ..initialize(configuration: _config(port, 'first'));
    await expectLater(
      client.getPresignedUrl('key', expirationSeconds: 0),
      throwsRangeError,
    );
    client.close();
    await expectLater(client.listObjects(), throwsStateError);
    client.close();
  });

  group('headObject', () {
    late S3Client client;
    setUp(() {
      client = S3Client()..initialize(configuration: _config(port, 'first'));
      addTearDown(client.close);
    });

    test('returns typed metadata', () async {
      final meta = await client.headObject('full');
      expect(meta.contentLength, 5);
      expect(meta.contentType, 'text/plain');
      expect(meta.eTag, '"abc"');
      expect(meta.lastModified, DateTime.utc(2006, 1, 2, 15, 4, 5));
      expect(meta.userMetadata, {'owner': 'me'});
      expect(meta.versionId, 'v1');
    });

    test('preserves missing values', () async {
      final meta = await client.headObject('sparse');
      expect(meta.eTag, isNull);
      expect(meta.lastModified, isNull);
      expect(meta.versionId, isNull);
      expect(meta.userMetadata, isEmpty);
    });

    test('distinguishes not found, permission, and other errors', () async {
      await expectLater(
        client.headObject('missing'),
        throwsA(isA<S3NotFoundException>()),
      );
      await expectLater(
        client.headObject('denied'),
        throwsA(
          isA<S3AccessDeniedException>().having(
            (e) => e.statusCode,
            'statusCode',
            403,
          ),
        ),
      );
      await expectLater(
        client.headObject('broken'),
        throwsA(
          isA<S3Exception>()
              .having((e) => e is S3NotFoundException, 'notFound', isFalse)
              .having((e) => e is S3AccessDeniedException, 'denied', isFalse),
        ),
      );
    });

    test('rejects an empty key', () async {
      await expectLater(client.headObject(''), throwsArgumentError);
    });
  });

  group('listObjectsPage', () {
    late S3Client client;
    setUp(() {
      client = S3Client()..initialize(configuration: _config(port, 'first'));
      addTearDown(client.close);
    });

    test('returns an empty page', () async {
      final page = await client.listObjectsPage(prefix: 'empty/');
      expect(page.objects, isEmpty);
      expect(page.commonPrefixes, isEmpty);
      expect(page.nextContinuationToken, isNull);
      expect(page.hasMore, isFalse);
    });

    test('follows continuation tokens across pages', () async {
      final keys = <String>[];
      String? token;
      do {
        final page = await client.listObjectsPage(
          prefix: 'p/',
          pageSize: 1,
          continuationToken: token,
        );
        keys.addAll(page.objects.map((o) => o.key));
        token = page.nextContinuationToken;
      } while (token != null);
      expect(keys, ['p/1-of-1', 'p/2-of-1', 'p/3-of-1']);
    });

    test('returns summaries and common prefixes for a delimiter', () async {
      final page = await client.listObjectsPage(prefix: 'a/', delimiter: '/');
      expect(page.commonPrefixes, ['a/sub/']);
      final object = page.objects.single;
      expect(object.key, 'a/top.txt');
      expect(object.size, 7);
      expect(object.eTag, '"e"');
      expect(object.storageClass, 'STANDARD');
      expect(object.lastModified, DateTime.utc(2020, 1, 2, 3, 4, 5));
    });

    test('validates inputs', () async {
      await expectLater(client.listObjectsPage(pageSize: 0), throwsRangeError);
      await expectLater(
        client.listObjectsPage(pageSize: 1001),
        throwsRangeError,
      );
      await expectLater(
        client.listObjectsPage(continuationToken: ''),
        throwsArgumentError,
      );
    });

    test('reports service failures', () async {
      final failing = S3Client()
        ..initialize(configuration: _config(port, 'failure'));
      addTearDown(failing.close);
      await expectLater(
        failing.listObjectsPage(),
        throwsA(isA<S3Exception>()),
      );
    });
  });

  group('upload options and presigned PUT', () {
    late S3Client client;
    late File file;
    setUp(() {
      client = S3Client()..initialize(configuration: _config(port, 'first'));
      addTearDown(client.close);
      final dir = Directory.systemTemp.createTempSync('s3_client_test');
      addTearDown(() => dir.deleteSync(recursive: true));
      file = File('${dir.path}/f.txt')..writeAsStringSync('hello');
    });

    Future<Map<String, dynamic>> lastPut() async {
      final http = HttpClient();
      try {
        final request = await http.getUrl(
          Uri.parse('http://127.0.0.1:$port/__last_put'),
        );
        final response = await request.close();
        return jsonDecode(await utf8.decodeStream(response))
            as Map<String, dynamic>;
      } finally {
        http.close();
      }
    }

    test('upload without options keeps working', () async {
      expect(await client.upload(file.path, 'k'), 'k');
    });

    test('upload sends content type and metadata', () async {
      await client.upload(
        file.path,
        'k',
        options: const S3UploadOptions(
          contentType: 'text/plain',
          metadata: {'owner': 'me'},
        ),
      );
      final headers = await lastPut();
      expect(headers['content-type'], 'text/plain');
      expect(headers['x-amz-meta-owner'], 'me');
    });

    test('upload rejects invalid options', () async {
      for (final options in [
        const S3UploadOptions(contentType: ''),
        const S3UploadOptions(contentType: 'a\nb'),
        const S3UploadOptions(metadata: {'': 'v'}),
        const S3UploadOptions(metadata: {'a b': 'v'}),
        const S3UploadOptions(metadata: {'a': 'é'}),
        const S3UploadOptions(metadata: {'A': '1', 'a': '2'}),
        S3UploadOptions(metadata: {'k': 'x' * 2048}),
      ]) {
        await expectLater(
          client.upload(file.path, 'k', options: options),
          throwsArgumentError,
        );
      }
    });

    test('presigned PUT returns the URL and signed headers', () async {
      final request = await client.getPresignedPutUrl(
        'dir/k',
        expirationSeconds: 60,
        options: const S3UploadOptions(
          contentType: 'text/plain',
          metadata: {'owner': 'me'},
        ),
      );
      expect(request.method, 'PUT');
      expect(request.url, contains('/first/dir/k'));
      expect(request.url, contains('X-Amz-Signature='));
      expect(request.headers['x-amz-meta-owner'], 'me');
      expect(request.headers['content-type'], 'text/plain');
      expect(request.headers, isNot(contains('host')));

      // The returned URL and headers are accepted by the endpoint.
      final http = HttpClient();
      try {
        final put = await http.putUrl(Uri.parse(request.url));
        request.headers.forEach(put.headers.set);
        final response = await put.close();
        await response.drain<void>();
        expect(response.statusCode, 200);
      } finally {
        http.close();
      }
      expect((await lastPut())['x-amz-meta-owner'], 'me');
    });

    test('presigned PUT without options has no extra headers', () async {
      final request = await client.getPresignedPutUrl('k');
      expect(request.headers, isNot(contains('content-type')));
      expect(
        request.headers.keys.where((k) => k.startsWith('x-amz-meta-')),
        isEmpty,
      );
    });

    test('presigned PUT validates inputs', () async {
      await expectLater(
        client.getPresignedPutUrl('k', expirationSeconds: 0),
        throwsRangeError,
      );
      await expectLater(
        client.getPresignedPutUrl('k', expirationSeconds: 604801),
        throwsRangeError,
      );
      await expectLater(client.getPresignedPutUrl(''), throwsArgumentError);
      await expectLater(
        client.getPresignedPutUrl(
          'k',
          options: const S3UploadOptions(metadata: {'bad key': 'v'}),
        ),
        throwsArgumentError,
      );
    });
  });
}
