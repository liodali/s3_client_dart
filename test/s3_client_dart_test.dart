import 'dart:io';
import 'dart:isolate';

import 'package:s3_client_dart/s3_client_dart.dart';
import 'package:test/test.dart';

void _serve(SendPort ready) async {
  final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
  ready.send(server.port);
  await for (final request in server) {
    await request.drain<void>();
    if (request.method == 'GET' &&
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
      request.response.statusCode = 404;
      request.response.headers.set('x-amz-error-code', 'NotFound');
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
}
