package main

// This file holds the S3 operations behind the C ABI in main.go. It does not
// import "C" so that it can be covered by ordinary `go test` runs.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const (
	maxPageSize       = 1000
	maxPresignSeconds = 604800
	maxMetadataBytes  = 2048
)

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
// the provider error code and the HTTP status when they are available.
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
	case code == "NotFound" || code == "NoSuchKey" || code == "404" || (status == 404 && code == ""):
		kind = "not_found"
	case status == 401 || status == 403 || code == "AccessDenied" || code == "Forbidden" || code == "403":
		kind = "access_denied"
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

func headObject(ctx context.Context, b *bucket, key string) (*objectMeta, error) {
	if key == "" {
		return nil, errors.New("object key must not be empty")
	}
	out, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)})
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
