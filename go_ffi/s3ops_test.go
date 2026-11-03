package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testBucket(t *testing.T, handler http.HandlerFunc) *bucket {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	b, err := newBucket(server.URL, "bkt", "key", "secret", "", "us-east-1", "")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHeadObject(t *testing.T) {
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bkt/full":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "5")
			w.Header().Set("ETag", `"abc"`)
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.Header().Set("x-amz-meta-owner", "me")
			w.Header().Set("x-amz-version-id", "v1")
		case "/bkt/sparse":
		case "/bkt/denied":
			w.WriteHeader(http.StatusForbidden)
		case "/bkt/broken":
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ctx := context.Background()
	m, err := headObject(ctx, b, "full")
	if err != nil {
		t.Fatal(err)
	}
	if *m.ContentLength != 5 || *m.ContentType != "text/plain" || *m.ETag != `"abc"` || *m.VersionID != "v1" || m.Metadata["owner"] != "me" || *m.LastModified != "2006-01-02T15:04:05Z" {
		t.Fatalf("unexpected metadata: %+v", m)
	}
	m, err = headObject(ctx, b, "sparse")
	if err != nil {
		t.Fatal(err)
	}
	if m.ETag != nil || m.VersionID != nil || m.LastModified != nil || len(m.Metadata) != 0 {
		t.Fatalf("missing values must stay missing: %+v", m)
	}
	for key, want := range map[string]string{"missing": "not_found", "denied": "access_denied", "broken": ""} {
		_, err := headObject(ctx, b, key)
		if err == nil {
			t.Fatalf("%s: expected error", key)
		}
		if kind, _, _ := classify(err); kind != want {
			t.Fatalf("%s: kind %q, want %q", key, kind, want)
		}
	}
	if _, err := headObject(ctx, b, ""); err == nil {
		t.Fatal("empty key must be rejected")
	}
}

func TestListPage(t *testing.T) {
	var query map[string]string
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		query = map[string]string{}
		for k := range r.URL.Query() {
			query[k] = r.URL.Query().Get(k)
		}
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Query().Get("prefix") {
		case "empty/":
			io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`)
		default:
			io.WriteString(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>tok</NextContinuationToken>
<Contents><Key>a/1</Key><Size>3</Size><ETag>"e"</ETag><LastModified>2020-01-02T03:04:05.000Z</LastModified><StorageClass>STANDARD</StorageClass></Contents>
<CommonPrefixes><Prefix>a/b/</Prefix></CommonPrefixes></ListBucketResult>`)
		}
	})
	ctx := context.Background()
	page, err := listPage(ctx, b, "empty/", "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Objects) != 0 || len(page.CommonPrefixes) != 0 || page.NextContinuationToken != nil {
		t.Fatalf("expected empty page: %+v", page)
	}
	if _, ok := query["max-keys"]; ok {
		t.Fatal("max-keys must be omitted when page size is unset")
	}
	page, err = listPage(ctx, b, "a/", "/", 10, "prev")
	if err != nil {
		t.Fatal(err)
	}
	if query["prefix"] != "a/" || query["delimiter"] != "/" || query["max-keys"] != "10" || query["continuation-token"] != "prev" {
		t.Fatalf("unexpected query: %v", query)
	}
	if len(page.Objects) != 1 || page.Objects[0].Key != "a/1" || *page.Objects[0].Size != 3 || *page.Objects[0].StorageClass != "STANDARD" {
		t.Fatalf("unexpected objects: %+v", page.Objects)
	}
	if len(page.CommonPrefixes) != 1 || page.CommonPrefixes[0] != "a/b/" || *page.NextContinuationToken != "tok" {
		t.Fatalf("unexpected page: %+v", page)
	}
	for _, size := range []int32{-1, 1001} {
		if _, err := listPage(ctx, b, "", "", size, ""); err == nil {
			t.Fatalf("page size %d must be rejected", size)
		}
	}
}

func TestUploadSendsContentTypeAndMetadata(t *testing.T) {
	var header http.Header
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		io.Copy(io.Discard, r.Body)
	})
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := uploadFile(context.Background(), b, path, "k", "text/plain", map[string]string{"owner": "me"})
	if err != nil {
		t.Fatal(err)
	}
	if header.Get("Content-Type") != "text/plain" || header.Get("X-Amz-Meta-Owner") != "me" {
		t.Fatalf("unexpected headers: %v", header)
	}
	for _, bad := range []string{"a\nb", "a\x00b", "a\tb", "a\x7fb"} {
		if err := uploadFile(context.Background(), b, path, "k", bad, nil); err == nil {
			t.Fatalf("content type %q must be rejected", bad)
		}
	}
}

func TestPresignPut(t *testing.T) {
	b := testBucket(t, func(http.ResponseWriter, *http.Request) {})
	ctx := context.Background()
	out, err := presignPut(ctx, b, "dir/k", 60, "text/plain", map[string]string{"owner": "me"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Method != "PUT" || !strings.Contains(out.URL, "/bkt/dir/k") || !strings.Contains(out.URL, "X-Amz-Signature=") {
		t.Fatalf("unexpected request: %+v", out)
	}
	if out.Headers["content-type"] != "text/plain" || out.Headers["x-amz-meta-owner"] != "me" {
		t.Fatalf("missing signed headers: %v", out.Headers)
	}
	if _, ok := out.Headers["host"]; ok {
		t.Fatal("host must not be returned")
	}
	plain, err := presignPut(ctx, b, "k", 60, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain.Headers["content-type"]; ok {
		t.Fatalf("unexpected content-type: %v", plain.Headers)
	}
	for _, seconds := range []int32{0, -1, maxPresignSeconds + 1} {
		if _, err := presignPut(ctx, b, "k", seconds, "", nil); err == nil {
			t.Fatalf("expiration %d must be rejected", seconds)
		}
	}
	if _, err := presignPut(ctx, b, "", 60, "", nil); err == nil {
		t.Fatal("empty key must be rejected")
	}
}

func TestParseMetadata(t *testing.T) {
	if m, err := parseMetadata(""); err != nil || m != nil {
		t.Fatalf("empty input: %v %v", m, err)
	}
	if m, err := parseMetadata(`{"a":"b"}`); err != nil || m["a"] != "b" {
		t.Fatalf("valid input: %v %v", m, err)
	}
	bad := []string{`not json`, `{"":"v"}`, `{"a b":"v"}`, `{"a:b":"v"}`, `{"a(b":"v"}`, `{"a/b":"v"}`, `{"a@b":"v"}`, `{"a\"b":"v"}`, `{"é":"v"}`, "{\"a\":\"\\n\"}", `{"A":"1","a":"2"}`, `{"k":"` + strings.Repeat("x", maxMetadataBytes) + `"}`}
	for _, raw := range bad {
		if _, err := parseMetadata(raw); err == nil {
			t.Fatalf("%s must be rejected", raw)
		}
	}
}

func TestEncodeIncludesErrorKind(t *testing.T) {
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	_, err := headObject(context.Background(), b, "k")
	var r response
	if e := json.Unmarshal([]byte(encode(nil, err)), &r); e != nil {
		t.Fatal(e)
	}
	if r.Kind != "access_denied" || r.Status != 403 || r.Error == "" {
		t.Fatalf("unexpected envelope: %+v", r)
	}
}
