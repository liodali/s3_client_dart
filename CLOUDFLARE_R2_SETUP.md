# Cloudflare R2 Setup Guide

This guide helps you configure the S3 client for Cloudflare R2.

## Common Issues

### "invalid header field value for Authorization"

This error typically occurs when:
1. **Credentials contain whitespace/newlines** - Ensure your access key and secret key don't have trailing newlines
2. **Wrong endpoint format** - R2 requires a specific endpoint format
3. **Missing region** - R2 requires "auto" as the region

## Correct Configuration for Cloudflare R2

```dart
import 'package:s3_client_dart/s3_client_dart.dart';

void main() async {
  final client = S3Client();
  
  client.initialize(
    configuration: S3Configuration(
      // R2 endpoint format: https://<account_id>.r2.cloudflarestorage.com
      endpoint: 'https://YOUR_ACCOUNT_ID.r2.cloudflarestorage.com',
      bucketName: 'your-bucket-name',
      accessKeyId: 'YOUR_ACCESS_KEY_ID',  // No whitespace!
      secretAccessKey: 'YOUR_SECRET_KEY',  // No whitespace!
      sessionToken: '',  // Leave empty for R2
      accountId: 'YOUR_ACCOUNT_ID',  // Optional
      region: 'auto',  // IMPORTANT: R2 uses 'auto' as region
    ),
  );
  
  // Now you can use the client
  await client.upload('/path/to/file.txt', 'remote/file.txt');
  client.close();
}
```

## Getting Your R2 Credentials

1. Go to Cloudflare Dashboard → R2
2. Click "Manage R2 API Tokens"
3. Create a new API token with appropriate permissions
4. Copy the **Access Key ID** and **Secret Access Key**
5. Your **Account ID** is in the URL: `https://dash.cloudflare.com/<ACCOUNT_ID>/r2`

## Endpoint Format

The endpoint must be in this exact format:
```
https://<ACCOUNT_ID>.r2.cloudflarestorage.com
```

**Example:**
```
https://YOUR_ACCOUNT_ID.r2.cloudflarestorage.com
```

## Debugging Checklist

1. **Verify credentials have no whitespace:**
   ```dart
   final accessKey = 'YOUR_KEY'.trim();
   final secretKey = 'YOUR_SECRET'.trim();
   ```

2. **Check the endpoint format:**
   - Must start with `https://`
   - Must include your account ID
   - Must end with `.r2.cloudflarestorage.com`
   - No trailing slashes

3. **Verify region is set to 'auto':**
   ```dart
   region: 'auto',  // Not 'us-east-1' or other AWS regions
   ```

4. **Inspect the reported error:** S3 failures throw `S3Exception`. Avoid logging credentials.

## Example with Environment Variables

```dart
import 'package:dotenv/dotenv.dart';

void main() async {
  final env = DotEnv()..load(['.env']);
  
  final client = S3Client();
  
  client.initialize(
    configuration: S3Configuration(
      endpoint: env['R2_ENDPOINT']!.trim(),
      bucketName: env['R2_BUCKET_NAME']!.trim(),
      accessKeyId: env['R2_ACCESS_KEY_ID']!.trim(),
      secretAccessKey: env['R2_SECRET_ACCESS_KEY']!.trim(),
      sessionToken: '',
      accountId: env['R2_ACCOUNT_ID']?.trim() ?? '',
      region: 'auto',
    ),
  );
}
```

## .env File Example

```env
R2_ENDPOINT=https://YOUR_ACCOUNT_ID.r2.cloudflarestorage.com
R2_BUCKET_NAME=my-bucket
R2_ACCESS_KEY_ID=your_access_key_here
R2_SECRET_ACCESS_KEY=your_secret_key_here
R2_ACCOUNT_ID=YOUR_ACCOUNT_ID
```

## Rebuild the Go Library

After making changes to the Go code, rebuild:

```bash
cd go_ffi
./deploy.sh  # macOS
# or
./deploy.sh  # Linux
```

## Testing

```dart
try {
  final result = await client.upload('/path/to/test.txt', 'test.txt');
  if (result.isNotEmpty) {
    print('✅ Upload successful: $result');
  } else {
    print('❌ Upload failed');
  }
} catch (e) {
  print('Error: $e');
}
```

## Common R2 Differences from AWS S3

1. **Region**: Always use `'auto'` instead of AWS regions
2. **Path Style**: R2 requires path-style URLs (handled automatically in our code)
3. **Endpoint**: Must use R2-specific endpoint format
4. **Public Access**: R2 has different public access controls than S3

## Need Help?

If you're still having issues:
1. Check the `S3Exception` message
2. Verify your R2 API token has the correct permissions
3. Ensure your bucket name is correct
4. Try uploading a small test file first
