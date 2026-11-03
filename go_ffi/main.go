package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var clients sync.Map
var nextID atomic.Uint64

func result(value any, err error) *C.char {
	return C.CString(encode(value, err))
}
func get(id C.uint64_t) (*bucket, error) {
	v, ok := clients.Load(uint64(id))
	if !ok {
		return nil, fmt.Errorf("S3 client is closed or not initialized")
	}
	return v.(*bucket), nil
}

//export s3Free
func s3Free(p *C.char) { C.free(unsafe.Pointer(p)) }

//export s3Create
func s3Create(endpoint, name, key, secret, token, region, account *C.char) *C.char {
	b, err := newBucket(C.GoString(endpoint), C.GoString(name), C.GoString(key), C.GoString(secret), C.GoString(token), C.GoString(region), C.GoString(account))
	if err != nil {
		return result(nil, err)
	}
	id := nextID.Add(1)
	clients.Store(id, b)
	return result(id, nil)
}

//export s3Close
func s3Close(id C.uint64_t) { clients.Delete(uint64(id)) }

//export s3Upload
func s3Upload(id C.uint64_t, path, key, contentType, metadata *C.char) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	meta, err := parseMetadata(C.GoString(metadata))
	if err != nil {
		return result(nil, err)
	}
	err = uploadFile(context.Background(), b, C.GoString(path), C.GoString(key), C.GoString(contentType), meta)
	return result(C.GoString(key), err)
}

//export s3Head
func s3Head(id C.uint64_t, key *C.char) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	out, err := headObject(context.Background(), b, C.GoString(key))
	if err != nil {
		return result(nil, err)
	}
	return result(out, nil)
}

//export s3ListPage
func s3ListPage(id C.uint64_t, prefix, delimiter *C.char, pageSize C.int32_t, token *C.char) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	out, err := listPage(context.Background(), b, C.GoString(prefix), C.GoString(delimiter), int32(pageSize), C.GoString(token))
	if err != nil {
		return result(nil, err)
	}
	return result(out, nil)
}

//export s3PresignPut
func s3PresignPut(id C.uint64_t, key *C.char, seconds C.int32_t, contentType, metadata *C.char) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	meta, err := parseMetadata(C.GoString(metadata))
	if err != nil {
		return result(nil, err)
	}
	out, err := presignPut(context.Background(), b, C.GoString(key), int32(seconds), C.GoString(contentType), meta)
	if err != nil {
		return result(nil, err)
	}
	return result(out, nil)
}

//export s3List
func s3List(id C.uint64_t) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{Bucket: aws.String(b.name)})
	keys := []string{}
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return result(nil, err)
		}
		for _, object := range page.Contents {
			keys = append(keys, aws.ToString(object.Key))
		}
	}
	return result(keys, nil)
}

//export s3Delete
func s3Delete(id C.uint64_t, key *C.char) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	_, err = b.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(b.name), Key: aws.String(C.GoString(key))})
	return result("", err)
}

//export s3Download
func s3Download(id C.uint64_t, key, path *C.char) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	obj, err := b.client.GetObject(context.Background(), &s3.GetObjectInput{Bucket: aws.String(b.name), Key: aws.String(C.GoString(key))})
	if err != nil {
		return result(nil, err)
	}
	defer obj.Body.Close()
	f, err := os.Create(C.GoString(path))
	if err != nil {
		return result(nil, err)
	}
	_, copyErr := io.Copy(f, obj.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return result(nil, copyErr)
	}
	return result("", closeErr)
}

//export s3Presign
func s3Presign(id C.uint64_t, key *C.char, seconds C.int32_t) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	out, err := s3.NewPresignClient(b.client).PresignGetObject(context.Background(), &s3.GetObjectInput{Bucket: aws.String(b.name), Key: aws.String(C.GoString(key))}, func(o *s3.PresignOptions) { o.Expires = time.Duration(seconds) * time.Second })
	if err != nil {
		return result(nil, err)
	}
	return result(out.URL, nil)
}

//export s3Exists
func s3Exists(id C.uint64_t, key *C.char) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	_, err = b.client.HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: aws.String(b.name), Key: aws.String(C.GoString(key))})
	if err == nil {
		return result(true, nil)
	}
	if kind, _, _ := classify(err); kind == "not_found" {
		return result(false, nil)
	}
	return result(nil, err)
}
func main() {}
