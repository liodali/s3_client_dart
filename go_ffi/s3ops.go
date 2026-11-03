package main

// This file holds the S3 operations behind the C ABI in main.go. It does not
// import "C" so that it can be covered by ordinary `go test` runs.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const (
	maxPageSize       = 1000
	maxPresignSeconds = 604800
	maxMetadataBytes  = 2048

	// hardMaxTransferBytes is the hard upper cap for byte transfers. The Dart
	// layer applies its own (lower, configurable) default before crossing the
	// FFI boundary; the cap is enforced here as well so oversized transfers are
	// rejected even when the caller disables its local check.
	defaultMaxTransferBytes = 16 << 20  // 16 MiB, the Dart-side default
	hardMaxTransferBytes    = 256 << 20 // 256 MiB hard cap

	// maxSingleCopyBytes is AWS S3's limit for a single CopyObject request.
	maxSingleCopyBytes = 5 << 30
)

// typedError carries a coarse kind through encode, so a specific failure can
// surface as a distinct typed exception in Dart. The wrapped error still
// provides code and status for the envelope.
type typedError struct {
	kind string
	msg  string
	err  error
}

func (e *typedError) Error() string { return e.msg }

func (e *typedError) Unwrap() error { return e.err }

type bucket struct {
	name   string
	client *s3.Client
}

// response is the JSON envelope returned across the C ABI.
type response struct {
	Value  any    `json:"value,omitempty"`
	Error  string `json:"error,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Code   string `json:"code,omitempty"`
	Status int    `json:"status,omitempty"`
}

// encode builds the JSON envelope for a value or an error.
func encode(value any, err error) string {
	r := response{Value: value}
	if err != nil {
		r.Value = nil
		r.Error = err.Error()
		r.Kind, r.Code, r.Status = classify(err)
	}
	b, _ := json.Marshal(r)
	return string(b)
}

// classify maps an error to a coarse kind ("not_found", "access_denied" or ""),
// the provider error code and the HTTP status when they are available. A
// [typedError] marker overrides the kind so specific failures surface as
// distinct typed exceptions in Dart.
func classify(err error) (kind, code string, status int) {
	var re *awshttp.ResponseError
	if errors.As(err, &re) {
		status = re.HTTPStatusCode()
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		code = api.ErrorCode()
	}
	switch {
	case code == "NotFound" || code == "NoSuchKey" || code == "NoSuchVersion" || code == "404" || status == 404:
		kind = "not_found"
	case status == 401 || status == 403 || code == "AccessDenied" || code == "Forbidden" || code == "403":
		kind = "access_denied"
	case code == "InvalidArgument" && strings.Contains(strings.ToLower(err.Error()), "version id"):
		// Providers reject an unknown or malformed version id with 400
		// InvalidArgument; callers expect a missing-object signal.
		kind = "not_found"
	}
	var te *typedError
	if errors.As(err, &te) {
		kind = te.kind
	}
	return kind, code, status
}

func newBucket(endpoint, name, key, secret, token, region, account string) (*bucket, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = true
		o.Credentials = aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: key, SecretAccessKey: secret, SessionToken: token, AccountID: account, Source: "static"}, nil
		}))
	})
	return &bucket{name: name, client: client}, nil
}

// parseMetadata decodes a JSON object of user metadata. An empty string means none.
func parseMetadata(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("invalid metadata: %w", err)
	}
	if err := validateMetadata(m); err != nil {
		return nil, err
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}

func validateMetadata(m map[string]string) error {
	size := 0
	seen := map[string]bool{}
	for k, v := range m {
		if k == "" {
			return errors.New("invalid metadata: empty key")
		}
		lower := strings.ToLower(k)
		if seen[lower] {
			return fmt.Errorf("invalid metadata: duplicate key %q", k)
		}
		seen[lower] = true
		for _, r := range k {
			if !isTokenChar(r) {
				return fmt.Errorf("invalid metadata key %q: only HTTP token characters are allowed", k)
			}
		}
		for _, r := range v {
			if r < ' ' || r >= 0x7f {
				return fmt.Errorf("invalid metadata value for key %q", k)
			}
		}
		size += len(k) + len(v)
	}
	if size > maxMetadataBytes {
		return fmt.Errorf("metadata exceeds %d bytes", maxMetadataBytes)
	}
	return nil
}

// isTokenChar reports whether r is an RFC 9110 token character, the set
// allowed in HTTP header names.
func isTokenChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return r < 0x80 && strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}

func validateContentType(contentType string) error {
	for _, r := range contentType {
		if r < ' ' || r == 0x7f {
			return errors.New("invalid content type: control characters are not allowed")
		}
	}
	return nil
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

type objectMeta struct {
	ContentLength *int64            `json:"contentLength,omitempty"`
	ContentType   *string           `json:"contentType,omitempty"`
	ETag          *string           `json:"eTag,omitempty"`
	LastModified  *string           `json:"lastModified,omitempty"`
	VersionID     *string           `json:"versionId,omitempty"`
	Metadata      map[string]string `json:"metadata"`
}

func headObject(ctx context.Context, b *bucket, key, versionId string) (*objectMeta, error) {
	if key == "" {
		return nil, errors.New("object key must not be empty")
	}
	in := &s3.HeadObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)}
	if versionId != "" {
		in.VersionId = aws.String(versionId)
	}
	out, err := b.client.HeadObject(ctx, in)
	if err != nil {
		return nil, err
	}
	meta := out.Metadata
	if meta == nil {
		meta = map[string]string{}
	}
	return &objectMeta{
		ContentLength: out.ContentLength,
		ContentType:   out.ContentType,
		ETag:          out.ETag,
		LastModified:  formatTime(out.LastModified),
		VersionID:     out.VersionId,
		Metadata:      meta,
	}, nil
}

type objectSummary struct {
	Key          string  `json:"key"`
	Size         *int64  `json:"size,omitempty"`
	ETag         *string `json:"eTag,omitempty"`
	LastModified *string `json:"lastModified,omitempty"`
	StorageClass *string `json:"storageClass,omitempty"`
}

type listPageResult struct {
	Objects               []objectSummary `json:"objects"`
	CommonPrefixes        []string        `json:"commonPrefixes"`
	NextContinuationToken *string         `json:"nextContinuationToken,omitempty"`
}

// listPage fetches one page. Empty prefix, delimiter and token are omitted; a
// pageSize of zero lets the service choose its default.
func listPage(ctx context.Context, b *bucket, prefix, delimiter string, pageSize int32, token string) (*listPageResult, error) {
	if pageSize < 0 || pageSize > maxPageSize {
		return nil, fmt.Errorf("page size must be between 1 and %d", maxPageSize)
	}
	in := &s3.ListObjectsV2Input{Bucket: aws.String(b.name)}
	if prefix != "" {
		in.Prefix = aws.String(prefix)
	}
	if delimiter != "" {
		in.Delimiter = aws.String(delimiter)
	}
	if pageSize > 0 {
		in.MaxKeys = aws.Int32(pageSize)
	}
	if token != "" {
		in.ContinuationToken = aws.String(token)
	}
	out, err := b.client.ListObjectsV2(ctx, in)
	if err != nil {
		return nil, err
	}
	page := &listPageResult{Objects: []objectSummary{}, CommonPrefixes: []string{}}
	for _, o := range out.Contents {
		s := objectSummary{Key: aws.ToString(o.Key), Size: o.Size, ETag: o.ETag, LastModified: formatTime(o.LastModified)}
		if o.StorageClass != "" {
			class := string(o.StorageClass)
			s.StorageClass = &class
		}
		page.Objects = append(page.Objects, s)
	}
	for _, p := range out.CommonPrefixes {
		page.CommonPrefixes = append(page.CommonPrefixes, aws.ToString(p.Prefix))
	}
	if aws.ToBool(out.IsTruncated) && aws.ToString(out.NextContinuationToken) != "" {
		page.NextContinuationToken = out.NextContinuationToken
	}
	return page, nil
}

func uploadFile(ctx context.Context, b *bucket, path, key, contentType string, metadata map[string]string) error {
	if err := validateContentType(contentType); err != nil {
		return err
	}
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	in := &s3.PutObjectInput{Bucket: aws.String(b.name), Key: aws.String(key), Body: f}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if len(metadata) > 0 {
		in.Metadata = metadata
	}
	_, err = b.client.PutObject(ctx, in)
	return err
}

type presignedRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
}

func validateSeconds(seconds int32) error {
	if seconds <= 0 || seconds > maxPresignSeconds {
		return fmt.Errorf("expiration must be between 1 and %d seconds", maxPresignSeconds)
	}
	return nil
}

func presignPut(ctx context.Context, b *bucket, key string, seconds int32, contentType string, metadata map[string]string) (*presignedRequest, error) {
	if key == "" {
		return nil, errors.New("object key must not be empty")
	}
	if err := validateSeconds(seconds); err != nil {
		return nil, err
	}
	if err := validateContentType(contentType); err != nil {
		return nil, err
	}
	if err := validateMetadata(metadata); err != nil {
		return nil, err
	}
	in := &s3.PutObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if len(metadata) > 0 {
		in.Metadata = metadata
	}
	out, err := s3.NewPresignClient(b.client).PresignPutObject(ctx, in, func(o *s3.PresignOptions) { o.Expires = time.Duration(seconds) * time.Second })
	if err != nil {
		return nil, err
	}
	headers := callerHeaders(out.SignedHeader)
	// The SDK presigner does not sign Content-Type, so it is not part of
	// SignedHeader. Return it anyway: the uploader must send it for S3 to store
	// it, although the signature does not enforce its value.
	if contentType != "" {
		headers["content-type"] = contentType
	}
	return &presignedRequest{URL: out.URL, Method: out.Method, Headers: headers}, nil
}

// callerHeaders returns the signed headers a caller must send. Host is derived
// from the URL by HTTP clients, so it is omitted.
func callerHeaders(h http.Header) map[string]string {
	headers := map[string]string{}
	for name, values := range h {
		if strings.EqualFold(name, "host") || len(values) == 0 {
			continue
		}
		headers[strings.ToLower(name)] = strings.Join(values, ",")
	}
	return headers
}

// escapeCopySourceSegment percent-encodes every byte that is not an RFC 3986
// unreserved character, so keys with spaces, plus signs, percent signs,
// delimiters and non-ASCII characters survive the CopySource round trip.
func escapeCopySourceSegment(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// encodeCopySource builds the x-amz-copy-source value: bucket and each key
// segment are escaped while '/' separators are preserved, and an optional
// source version id is appended as ?versionId=.
func encodeCopySource(bucket, key, versionId string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = escapeCopySourceSegment(segment)
	}
	source := escapeCopySourceSegment(bucket) + "/" + strings.Join(segments, "/")
	if versionId != "" {
		source += "?versionId=" + url.QueryEscape(versionId)
	}
	return source
}

type copyResult struct {
	ETag                *string `json:"eTag,omitempty"`
	VersionID           *string `json:"versionId,omitempty"`
	LastModified        *string `json:"lastModified,omitempty"`
	CopySourceVersionID *string `json:"copySourceVersionId,omitempty"`
}

// copyObject copies one object. With replaceMetadata false the COPY directive
// keeps the source metadata; with true the REPLACE directive applies the
// supplied content type and metadata. Copies larger than the provider's
// single-request limit (5 GB on AWS S3) are reported as a typed
// unsupported_large_copy error; multipart copy is not implemented.
func copyObject(ctx context.Context, b *bucket, sourceBucket, sourceKey, destinationKey, sourceVersionId string, replaceMetadata bool, contentType string, metadata map[string]string) (*copyResult, error) {
	if sourceKey == "" || destinationKey == "" {
		return nil, errors.New("object key must not be empty")
	}
	if !replaceMetadata && (contentType != "" || len(metadata) > 0) {
		return nil, errors.New("metadata and content type are only accepted when replaceMetadata is true")
	}
	if err := validateContentType(contentType); err != nil {
		return nil, err
	}
	if err := validateMetadata(metadata); err != nil {
		return nil, err
	}
	if sourceBucket == "" {
		sourceBucket = b.name
	}
	in := &s3.CopyObjectInput{
		Bucket:     aws.String(b.name),
		Key:        aws.String(destinationKey),
		CopySource: aws.String(encodeCopySource(sourceBucket, sourceKey, sourceVersionId)),
	}
	if replaceMetadata {
		in.MetadataDirective = types.MetadataDirectiveReplace
		if contentType != "" {
			in.ContentType = aws.String(contentType)
		}
		if len(metadata) > 0 {
			in.Metadata = metadata
		}
	}
	out, err := b.client.CopyObject(ctx, in)
	if err != nil {
		return nil, maybeLargeCopyError(err)
	}
	res := &copyResult{VersionID: out.VersionId, CopySourceVersionID: out.CopySourceVersionId}
	if out.CopyObjectResult != nil {
		res.ETag = out.CopyObjectResult.ETag
		res.LastModified = formatTime(out.CopyObjectResult.LastModified)
	}
	return res, nil
}

// maybeLargeCopyError tags the provider error as unsupported_large_copy when
// the copy source exceeds the single-request copy limit, so Dart can throw a
// dedicated exception instead of a generic one.
func maybeLargeCopyError(err error) error {
	var api smithy.APIError
	if !errors.As(err, &api) || api.ErrorCode() != "InvalidRequest" {
		return err
	}
	msg := strings.ToLower(api.ErrorMessage())
	if !strings.Contains(msg, "larger than") && !strings.Contains(msg, "5 gb") {
		return err
	}
	return &typedError{kind: "unsupported_large_copy", msg: err.Error(), err: err}
}

type objectIdentifier struct {
	Key       string `json:"key"`
	VersionID string `json:"versionId,omitempty"`
}

type deleteObjectsPayload struct {
	Items []objectIdentifier `json:"items"`
}

func parseDeletePayload(raw string) ([]objectIdentifier, error) {
	if raw == "" {
		return nil, nil
	}
	var p deleteObjectsPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("invalid delete payload: %w", err)
	}
	return p.Items, nil
}

type deletedObject struct {
	Key       string  `json:"key"`
	VersionID *string `json:"versionId,omitempty"`
}

type deleteError struct {
	Key       string  `json:"key"`
	VersionID *string `json:"versionId,omitempty"`
	Code      string  `json:"code,omitempty"`
	Message   string  `json:"message,omitempty"`
}

type deleteObjectsResult struct {
	Deleted []deletedObject `json:"deleted"`
	Errors  []deleteError   `json:"errors"`
}

// deleteObjects removes up to maxPageSize identifiers per request, splitting
// larger lists into chunks. Per-key failures inside a successful chunk come
// back in Errors; if a whole chunk request fails, every key of that chunk is
// recorded as an error and the remaining chunks still run. Duplicate keys are
// sent as given and never deduplicated.
func deleteObjects(ctx context.Context, b *bucket, ids []objectIdentifier) (*deleteObjectsResult, error) {
	for _, id := range ids {
		if id.Key == "" {
			return nil, errors.New("object key must not be empty")
		}
	}
	res := &deleteObjectsResult{Deleted: []deletedObject{}, Errors: []deleteError{}}
	for start := 0; start < len(ids); start += maxPageSize {
		end := start + maxPageSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		objects := make([]types.ObjectIdentifier, len(chunk))
		for i, id := range chunk {
			objects[i] = types.ObjectIdentifier{Key: aws.String(id.Key)}
			if id.VersionID != "" {
				objects[i].VersionId = aws.String(id.VersionID)
			}
		}
		out, err := b.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(b.name),
			// Quiet=false keeps the deleted list in the response so every
			// chunk reports both its successes and its failures.
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(false)},
		})
		if err != nil {
			code := ""
			var api smithy.APIError
			if errors.As(err, &api) {
				code = api.ErrorCode()
			}
			for _, id := range chunk {
				vid := id.VersionID
				res.Errors = append(res.Errors, deleteError{Key: id.Key, VersionID: nonEmpty(vid), Code: code, Message: err.Error()})
			}
			continue
		}
		for _, d := range out.Deleted {
			res.Deleted = append(res.Deleted, deletedObject{Key: aws.ToString(d.Key), VersionID: nonEmpty(aws.ToString(d.VersionId))})
		}
		for _, e := range out.Errors {
			res.Errors = append(res.Errors, deleteError{
				Key:       aws.ToString(e.Key),
				VersionID: nonEmpty(aws.ToString(e.VersionId)),
				Code:      aws.ToString(e.Code),
				Message:   aws.ToString(e.Message),
			})
		}
	}
	return res, nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type objectVersion struct {
	Key            string  `json:"key"`
	VersionID      string  `json:"versionId"`
	IsLatest       bool    `json:"isLatest"`
	IsDeleteMarker bool    `json:"isDeleteMarker"`
	LastModified   *string `json:"lastModified,omitempty"`
	ETag           *string `json:"eTag,omitempty"`
	Size           *int64  `json:"size,omitempty"`
}

type versionsPage struct {
	Versions            []objectVersion `json:"versions"`
	NextKeyMarker       *string         `json:"nextKeyMarker,omitempty"`
	NextVersionIDMarker *string         `json:"nextVersionIdMarker,omitempty"`
}

// listVersions fetches one page of object versions and delete markers.
// Providers without versioning support return the provider error unchanged.
func listVersions(ctx context.Context, b *bucket, prefix string, pageSize int32, keyMarker, versionIdMarker string) (*versionsPage, error) {
	if pageSize < 0 || pageSize > maxPageSize {
		return nil, fmt.Errorf("page size must be between 1 and %d", maxPageSize)
	}
	in := &s3.ListObjectVersionsInput{Bucket: aws.String(b.name)}
	if prefix != "" {
		in.Prefix = aws.String(prefix)
	}
	if pageSize > 0 {
		in.MaxKeys = aws.Int32(pageSize)
	}
	if keyMarker != "" {
		in.KeyMarker = aws.String(keyMarker)
	}
	if versionIdMarker != "" {
		in.VersionIdMarker = aws.String(versionIdMarker)
	}
	out, err := b.client.ListObjectVersions(ctx, in)
	if err != nil {
		return nil, err
	}
	page := &versionsPage{Versions: []objectVersion{}}
	for _, v := range out.Versions {
		page.Versions = append(page.Versions, objectVersion{
			Key:          aws.ToString(v.Key),
			VersionID:    aws.ToString(v.VersionId),
			IsLatest:     aws.ToBool(v.IsLatest),
			LastModified: formatTime(v.LastModified),
			ETag:         v.ETag,
			Size:         v.Size,
		})
	}
	for _, d := range out.DeleteMarkers {
		page.Versions = append(page.Versions, objectVersion{
			Key:            aws.ToString(d.Key),
			VersionID:      aws.ToString(d.VersionId),
			IsLatest:       aws.ToBool(d.IsLatest),
			IsDeleteMarker: true,
			LastModified:   formatTime(d.LastModified),
		})
	}
	if aws.ToBool(out.IsTruncated) && aws.ToString(out.NextKeyMarker) != "" {
		page.NextKeyMarker = out.NextKeyMarker
		if aws.ToString(out.NextVersionIdMarker) != "" {
			page.NextVersionIDMarker = out.NextVersionIdMarker
		}
	}
	return page, nil
}

func deleteObject(ctx context.Context, b *bucket, key, versionId string) error {
	if key == "" {
		return errors.New("object key must not be empty")
	}
	in := &s3.DeleteObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)}
	if versionId != "" {
		in.VersionId = aws.String(versionId)
	}
	_, err := b.client.DeleteObject(ctx, in)
	return err
}

func uploadBytes(ctx context.Context, b *bucket, key, contentType string, metadata map[string]string, data []byte) error {
	if key == "" {
		return errors.New("object key must not be empty")
	}
	if err := validateContentType(contentType); err != nil {
		return err
	}
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	if len(data) > hardMaxTransferBytes {
		return &typedError{
			kind: "size_limit",
			msg:  fmt.Sprintf("upload of %d bytes exceeds the %d byte limit", len(data), hardMaxTransferBytes),
		}
	}
	in := &s3.PutObjectInput{Bucket: aws.String(b.name), Key: aws.String(key), Body: bytes.NewReader(data)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if len(metadata) > 0 {
		in.Metadata = metadata
	}
	_, err := b.client.PutObject(ctx, in)
	return err
}

func downloadBytes(ctx context.Context, b *bucket, key, versionId string, maxBytes int64) ([]byte, error) {
	if key == "" {
		return nil, errors.New("object key must not be empty")
	}
	if maxBytes <= 0 || maxBytes > hardMaxTransferBytes {
		return nil, fmt.Errorf("max bytes must be between 1 and %d", hardMaxTransferBytes)
	}
	in := &s3.GetObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)}
	if versionId != "" {
		in.VersionId = aws.String(versionId)
	}
	obj, err := b.client.GetObject(ctx, in)
	if err != nil {
		return nil, err
	}
	defer obj.Body.Close()
	if obj.ContentLength != nil && *obj.ContentLength > maxBytes {
		return nil, &typedError{
			kind: "size_limit",
			msg:  fmt.Sprintf("object is %d bytes, exceeding the %d byte limit", *obj.ContentLength, maxBytes),
		}
	}
	// Read one byte more than the limit to detect an object whose true size
	// exceeds it even without a Content-Length.
	data, err := io.ReadAll(io.LimitReader(obj.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, &typedError{
			kind: "size_limit",
			msg:  fmt.Sprintf("object exceeds the %d byte limit", maxBytes),
		}
	}
	return data, nil
}

// presignGet signs a GET URL, optionally for a specific version.
func presignGet(ctx context.Context, b *bucket, key string, seconds int32, versionId string) (string, error) {
	if key == "" {
		return "", errors.New("object key must not be empty")
	}
	if err := validateSeconds(seconds); err != nil {
		return "", err
	}
	in := &s3.GetObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)}
	if versionId != "" {
		in.VersionId = aws.String(versionId)
	}
	out, err := s3.NewPresignClient(b.client).PresignGetObject(ctx, in, func(o *s3.PresignOptions) { o.Expires = time.Duration(seconds) * time.Second })
	if err != nil {
		return "", err
	}
	return out.URL, nil
}
