package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type bucket struct {
	name   string
	client *s3.Client
}

var clients sync.Map
var nextID atomic.Uint64

type response struct {
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

func result(value any, err error) *C.char {
	r := response{Value: value}
	if err != nil {
		r.Error = err.Error()
	}
	b, _ := json.Marshal(r)
	return C.CString(string(b))
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
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(C.GoString(region)))
	if err != nil {
		return result(nil, err)
	}
	endpointValue, keyValue, secretValue := C.GoString(endpoint), C.GoString(key), C.GoString(secret)
	tokenValue, accountValue := C.GoString(token), C.GoString(account)
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpointValue != "" {
			o.BaseEndpoint = aws.String(endpointValue)
		}
		o.UsePathStyle = true
		o.Credentials = aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: keyValue, SecretAccessKey: secretValue, SessionToken: tokenValue, AccountID: accountValue, Source: "static"}, nil
		}))
	})
	id := nextID.Add(1)
	clients.Store(id, &bucket{name: C.GoString(name), client: client})
	return result(id, nil)
}

//export s3Close
func s3Close(id C.uint64_t) { clients.Delete(uint64(id)) }

//export s3Upload
func s3Upload(id C.uint64_t, path, key *C.char) *C.char {
	b, err := get(id)
	if err != nil {
		return result(nil, err)
	}
	f, err := os.Open(C.GoString(path))
	if err != nil {
		return result(nil, err)
	}
	defer f.Close()
	_, err = b.client.PutObject(context.Background(), &s3.PutObjectInput{Bucket: aws.String(b.name), Key: aws.String(C.GoString(key)), Body: f})
	return result(C.GoString(key), err)
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
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "404") {
		return result(false, nil)
	}
	return result(nil, err)
}
func main() {}
