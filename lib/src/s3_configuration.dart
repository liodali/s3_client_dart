class S3Configuration {
  final String endpoint;
  final String bucketName;
  final String accessKeyId;
  final String secretAccessKey;
  final String sessionToken;
  final String accountId;
  final String region;

  S3Configuration({
    required this.endpoint,
    required this.bucketName,
    required this.accessKeyId,
    required this.secretAccessKey,
    required this.sessionToken,
    required this.accountId,
    required this.region,
  });
}
