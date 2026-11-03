package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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
	m, err := headObject(ctx, b, "full", "")
	if err != nil {
		t.Fatal(err)
	}
	if *m.ContentLength != 5 || *m.ContentType != "text/plain" || *m.ETag != `"abc"` || *m.VersionID != "v1" || m.Metadata["owner"] != "me" || *m.LastModified != "2006-01-02T15:04:05Z" {
		t.Fatalf("unexpected metadata: %+v", m)
	}
	m, err = headObject(ctx, b, "sparse", "")
	if err != nil {
		t.Fatal(err)
	}
	if m.ETag != nil || m.VersionID != nil || m.LastModified != nil || len(m.Metadata) != 0 {
		t.Fatalf("missing values must stay missing: %+v", m)
	}
	for key, want := range map[string]string{"missing": "not_found", "denied": "access_denied", "broken": ""} {
		_, err := headObject(ctx, b, key, "")
		if err == nil {
			t.Fatalf("%s: expected error", key)
		}
		if kind, _, _ := classify(err); kind != want {
			t.Fatalf("%s: kind %q, want %q", key, kind, want)
		}
	}
	if _, err := headObject(ctx, b, "", ""); err == nil {
		t.Fatal("empty key must be rejected")
	}
}

func TestHeadObjectVersion(t *testing.T) {
	var versionQuery string
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		versionQuery = r.URL.Query().Get("versionId")
		if versionQuery == "missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("x-amz-version-id", "v9")
	})
	ctx := context.Background()
	m, err := headObject(ctx, b, "k", "v9")
	if err != nil {
		t.Fatal(err)
	}
	if versionQuery != "v9" || m.VersionID == nil || *m.VersionID != "v9" {
		t.Fatalf("version id not sent: %q %+v", versionQuery, m)
	}
	if _, err := headObject(ctx, b, "k", "missing"); err == nil {
		t.Fatal("missing version must fail")
	} else if kind, _, _ := classify(err); kind != "not_found" {
		t.Fatalf("kind %q, want not_found", kind)
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
	_, err := headObject(context.Background(), b, "k", "")
	var r response
	if e := json.Unmarshal([]byte(encode(nil, err)), &r); e != nil {
		t.Fatal(e)
	}
	if r.Kind != "access_denied" || r.Status != 403 || r.Error == "" {
		t.Fatalf("unexpected envelope: %+v", r)
	}
}

func TestEncodeCopySource(t *testing.T) {
	got := encodeCopySource("bkt", "dir with space/a+b%c/é&=.txt", "v 1")
	want := "bkt/dir%20with%20space/a%2Bb%25c/%C3%A9%26%3D.txt?versionId=v+1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := encodeCopySource("bkt", "plain", ""); got != "bkt/plain" {
		t.Fatalf("plain source: %q", got)
	}
}

func TestCopyObject(t *testing.T) {
	var source, directive, contentType, meta string
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		source = r.Header.Get("X-Amz-Copy-Source")
		directive = r.Header.Get("X-Amz-Metadata-Directive")
		contentType = r.Header.Get("Content-Type")
		meta = r.Header.Get("X-Amz-Meta-Owner")
		if strings.Contains(source, "huge") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `<Error><Code>InvalidRequest</Code><Message>The specified copy source is larger than 5 GB</Message></Error>`)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, `<CopyObjectResult><ETag>"e"</ETag><LastModified>2020-01-02T03:04:05.000Z</LastModified></CopyObjectResult>`)
	})
	ctx := context.Background()
	out, err := copyObject(ctx, b, "other", "src/a b", "dst", "", false, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if source != "other/src/a%20b" || directive != "" || *out.ETag != `"e"` {
		t.Fatalf("unexpected copy: %q %q %+v", source, directive, out)
	}
	_, err = copyObject(ctx, b, "", "src", "dst", "", true, "text/plain", map[string]string{"owner": "me"})
	if err != nil {
		t.Fatal(err)
	}
	if directive != "REPLACE" || contentType != "text/plain" || meta != "me" {
		t.Fatalf("replace directive not applied: %q %q %q", directive, contentType, meta)
	}
	if _, err := copyObject(ctx, b, "", "src", "dst", "", false, "text/plain", nil); err == nil {
		t.Fatal("options without replace must be rejected")
	}
	_, err = copyObject(ctx, b, "", "huge", "dst", "", false, "", nil)
	if err == nil {
		t.Fatal("large copy must fail")
	}
	if kind, _, _ := classify(err); kind != "unsupported_large_copy" {
		t.Fatalf("kind %q, want unsupported_large_copy", kind)
	}
}

func TestDeleteObjectsChunkingAndPartialFailures(t *testing.T) {
	var batches []int
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		keys := regexp.MustCompile(`<Key>([^<]*)</Key>`).FindAllStringSubmatch(string(body), -1)
		batches = append(batches, len(keys))
		w.Header().Set("Content-Type", "application/xml")
		var out strings.Builder
		out.WriteString(`<DeleteResult>`)
		for _, k := range keys {
			key := k[1]
			if key == "denied" {
				out.WriteString(`<Error><Key>denied</Key><Code>AccessDenied</Code><Message>no</Message></Error>`)
			} else {
				out.WriteString(`<Deleted><Key>` + key + `</Key></Deleted>`)
			}
		}
		out.WriteString(`</DeleteResult>`)
		io.WriteString(w, out.String())
	})
	ids := make([]objectIdentifier, 0, 1001)
	for i := 0; i < 1000; i++ {
		ids = append(ids, objectIdentifier{Key: fmt.Sprintf("k%d", i)})
	}
	ids = append(ids, objectIdentifier{Key: "denied"})
	out, err := deleteObjects(context.Background(), b, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 || batches[0] != 1000 || batches[1] != 1 {
		t.Fatalf("unexpected batches: %v", batches)
	}
	if len(out.Deleted) != 1000 || len(out.Errors) != 1 || out.Errors[0].Key != "denied" || out.Errors[0].Code != "AccessDenied" {
		t.Fatalf("unexpected result: %d deleted, %+v", len(out.Deleted), out.Errors)
	}
	if _, err := deleteObjects(context.Background(), b, []objectIdentifier{{Key: ""}}); err == nil {
		t.Fatal("empty key must be rejected")
	}
}

func TestDeleteObjectsChunkRequestFailure(t *testing.T) {
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>no</Message></Error>`)
	})
	out, err := deleteObjects(context.Background(), b, []objectIdentifier{{Key: "a"}, {Key: "b"}})
	if err != nil {
		t.Fatalf("chunk failures must be reported per key: %v", err)
	}
	if len(out.Deleted) != 0 || len(out.Errors) != 2 || out.Errors[0].Code != "AccessDenied" {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestListVersions(t *testing.T) {
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/xml")
		if q.Get("key-marker") == "" {
			io.WriteString(w, `<ListVersionsResult><IsTruncated>true</IsTruncated><NextKeyMarker>k2</NextKeyMarker><NextVersionIdMarker>v2</NextVersionIdMarker>
<Version><Key>k1</Key><VersionId>v1</VersionId><IsLatest>true</IsLatest><ETag>"e"</ETag><Size>4</Size><LastModified>2020-01-02T03:04:05.000Z</LastModified></Version>
<DeleteMarker><Key>k1</Key><VersionId>v0</VersionId><IsLatest>false</IsLatest></DeleteMarker></ListVersionsResult>`)
			return
		}
		io.WriteString(w, `<ListVersionsResult><IsTruncated>false</IsTruncated></ListVersionsResult>`)
	})
	ctx := context.Background()
	page, err := listVersions(ctx, b, "p/", 10, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Versions) != 2 || page.Versions[0].VersionID != "v1" || !page.Versions[0].IsLatest || page.Versions[1].IsDeleteMarker != true {
		t.Fatalf("unexpected versions: %+v", page.Versions)
	}
	if *page.NextKeyMarker != "k2" || *page.NextVersionIDMarker != "v2" {
		t.Fatalf("unexpected markers: %+v", page)
	}
	last, err := listVersions(ctx, b, "p/", 10, "k2", "v2")
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Versions) != 0 || last.NextKeyMarker != nil {
		t.Fatalf("unexpected last page: %+v", last)
	}
}

func TestUploadAndDownloadBytesLimits(t *testing.T) {
	b := testBucket(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Length", "5")
			io.WriteString(w, "hello")
			return
		}
		io.Copy(io.Discard, r.Body)
	})
	ctx := context.Background()
	if err := uploadBytes(ctx, b, "k", "text/plain", nil, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := uploadBytes(ctx, b, "k", "", nil, make([]byte, hardMaxTransferBytes+1)); err == nil {
		t.Fatal("oversized upload must be rejected")
	} else if kind, _, _ := classify(err); kind != "size_limit" {
		t.Fatalf("kind %q, want size_limit", kind)
	}
	data, err := downloadBytes(ctx, b, "k", "", 16)
	if err != nil || string(data) != "hello" {
		t.Fatalf("download: %q %v", data, err)
	}
	if _, err := downloadBytes(ctx, b, "k", "", 4); err == nil {
		t.Fatal("download above the limit must fail")
	} else if kind, _, _ := classify(err); kind != "size_limit" {
		t.Fatalf("kind %q, want size_limit", kind)
	}
	if _, err := downloadBytes(ctx, b, "k", "", 0); err == nil {
		t.Fatal("non-positive limit must be rejected")
	}
}
