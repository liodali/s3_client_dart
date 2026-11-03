/// A Dart client for S3-compatible storage using the AWS SDK for Go.
library;

export 'src/s3_client_dart_base.dart' show S3Client;
export 'src/s3_exceptions.dart'
    show
        S3AccessDeniedException,
        S3Exception,
        S3NotFoundException,
        S3SizeLimitException,
        S3UnsupportedLargeCopyException;
export 'src/s3_models.dart'
    show
        S3BatchDeleteResult,
        S3CopyMetadataBehavior,
        S3CopyResult,
        S3DeleteError,
        S3DeletedObject,
        S3ListPage,
        S3ListVersionsPage,
        S3ObjectMetadata,
        S3ObjectIdentifier,
        S3ObjectSummary,
        S3ObjectVersion,
        S3PresignedRequest,
        S3UploadOptions;
export 'src/s3_configuration.dart' show S3Configuration;
