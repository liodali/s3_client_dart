/// A Dart client for S3-compatible storage using the AWS SDK for Go.
library;

export 'src/s3_client_dart_base.dart' show S3Client;
export 'src/s3_exceptions.dart'
    show S3AccessDeniedException, S3Exception, S3NotFoundException;
export 'src/s3_models.dart'
    show
        S3ListPage,
        S3ObjectMetadata,
        S3ObjectSummary,
        S3PresignedRequest,
        S3UploadOptions;
export 'src/s3_configuration.dart' show S3Configuration;
