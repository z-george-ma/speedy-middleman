package lib

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

type HttpClient struct {
	*http.Client
}

func (hc *HttpClient) Init(timeoutSec int, maxIdleConns int, connsPerHost int) {
	t := http.DefaultTransport.(*http.Transport).Clone()

	// TODO add DNS cache
	t.MaxIdleConns = maxIdleConns
	t.MaxConnsPerHost = connsPerHost
	t.MaxIdleConnsPerHost = connsPerHost

	hc.Client = &http.Client{ // alloc
		Timeout:   time.Second * time.Duration(timeoutSec),
		Transport: t,
	}
}

type HttpRequestBody struct {
	value       io.Reader
	contentType string
	compressed  bool
}

func Body(value any, contentType string, compress bool) (ret HttpRequestBody) {
	if value == nil {
		return HttpRequestBody{}
	}

	var rawValue []byte
	ret.contentType = contentType

	switch value.(type) {
	case string:
		rawValue = StringToBytes(value.(string))
	case []byte:
		rawValue = value.([]byte)
	}

	if rawValue == nil {
		if ret.contentType == "" {
			ret.contentType = "application/json; charset=utf-8"
		}

		if strings.HasPrefix(strings.ToLower(contentType), "application/json") {
			var err error
			rawValue, err = json.Marshal(value)

			if err != nil {
				panic(err)
			}
		} else {
			panic("Unsupported content type") // alloc
		}
	}

	if !compress {
		ret.value = bytes.NewReader(rawValue)
		return
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write(rawValue)
	gz.Close()
	ret.value = &buf
	ret.compressed = true
	return
}

func NewHttpRequest(ctx context.Context, method string, url string, body HttpRequestBody, headers map[string]string) *http.Request {
	req, err := http.NewRequestWithContext(ctx, method, url, body.value)

	if err != nil {
		return nil
	}

	if headers != nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	if body.contentType != "" {
		req.Header.Set("content-type", body.contentType)
	}

	if body.compressed {
		req.Header.Set("content-encoding", "gzip")
	}

	return req
}

type ResponseCache struct {
	Status           string // e.g. "200 OK"
	StatusCode       int    // e.g. 200
	Proto            string // e.g. "HTTP/1.0"
	ProtoMajor       int    // e.g. 1
	ProtoMinor       int    // e.g. 0
	Header           map[string][]string
	Body             []byte
	ContentLength    int64
	TransferEncoding []string
	Uncompressed     bool
}

var cachedRequestPool *sync.Pool = &sync.Pool{
	New: func() any {
		return make([]byte, 0, 4096)
	},
}

type bufferReadCloser struct {
	rawBuf       []byte
	buffer       *bytes.Buffer
	responseBody io.Closer
}

func (br bufferReadCloser) Read(b []byte) (n int, err error) {
	return br.buffer.Read(b)
}

func (br bufferReadCloser) Close() error {
	if br.rawBuf != nil {
		cachedRequestPool.Put(br.rawBuf)
	}

	if br.responseBody != nil {
		return br.responseBody.Close()
	}
	return nil
}

func newBufferReadCloser(buf []byte, close io.Closer) bufferReadCloser {
	var rawBuf []byte
	if buf == nil {
		rawBuf = cachedRequestPool.Get().([]byte)
		buf = rawBuf
	}
	return bufferReadCloser{
		rawBuf:       rawBuf,
		buffer:       bytes.NewBuffer(buf),
		responseBody: close,
	}
}

func CachedRequest(request func() (*http.Response, error), pathElem ...string) (ret *http.Response, err error) {
	file := path.Join(pathElem...)
	f, err := os.Open(file)
	if err == nil {
		defer f.Close()

		dec := gob.NewDecoder(f)

		var cache ResponseCache
		err = dec.Decode(&cache)

		if err != nil {
			return
		}

		ret = &http.Response{
			Status:           cache.Status,
			StatusCode:       cache.StatusCode,
			Proto:            cache.Proto,
			ProtoMajor:       cache.ProtoMajor,
			ProtoMinor:       cache.ProtoMinor,
			Header:           cache.Header,
			Body:             newBufferReadCloser(cache.Body, nil),
			ContentLength:    cache.ContentLength,
			TransferEncoding: cache.TransferEncoding,
			Uncompressed:     cache.Uncompressed,
		}
		return
	}

	ret, err = request()
	if err != nil {
		return
	}

	buf := newBufferReadCloser(nil, ret.Body)
	_, err = io.Copy(buf.buffer, ret.Body)

	if err != nil {
		buf.Close()
		return
	}

	cache := ResponseCache{
		Status:           ret.Status,
		StatusCode:       ret.StatusCode,
		Proto:            ret.Proto,
		ProtoMajor:       ret.ProtoMajor,
		ProtoMinor:       ret.ProtoMinor,
		Header:           ret.Header,
		Body:             buf.buffer.Bytes(),
		ContentLength:    ret.ContentLength,
		TransferEncoding: ret.TransferEncoding,
		Uncompressed:     ret.Uncompressed,
	}

	f, err = CreateFile(pathElem...)
	if err != nil {
		buf.Close()
		return
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	err = enc.Encode(cache)

	if err != nil {
		buf.Close()
		return
	}

	ret.Body = buf
	return
}

func UnzipResponse(res *http.Response) (ret []byte, err error) {
	buf := newBufferReadCloser(nil, res.Body)

	reader, err := gzip.NewReader(res.Body)
	if err != nil {
		return
	}
	defer reader.Close()

	_, err = io.Copy(buf.buffer, reader)
	if err != nil {
		buf.Close()
		return
	}

	res.Body = buf
	ret = buf.buffer.Bytes()
	return
}
