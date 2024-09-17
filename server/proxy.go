package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"lib"
	"net"
	"reflect"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

type HttpRequest struct {
	Method  string
	Url     string
	Version string
	Headers map[string]string
}

type HttpResponse struct {
	StatusCode int
	Reason     string
	Version    string
	Headers    map[string]string
}

var MaxHeadersSupported = 100
var ErrHttpMalformedHeader = errors.New("malformed HTTP Header")
var ErrExceedingHeaderCount = errors.New("exceeding max number of HTTP headers")

func parseHttp(r *bufio.Reader, parseStartLine func(string) error) (headers map[string]string, err error) {
	var b []byte

	startLine := true
	headers = map[string]string{}

	for {
		b, err = r.ReadSlice('\n')
		if err != nil {
			return
		}

		l := len(b)

		if b[l-2] != '\r' {
			err = ErrHttpMalformedHeader
			return
		}

		b = b[:l-2]

		bs := string(b)

		if startLine {
			if err = parseStartLine(bs); err != nil {
				return
			}

			startLine = false
			continue
		}

		if len(b) == 0 {
			// HTTP header parse complete
			break
		}

		s := strings.Split(bs, ": ")
		if len(s) != 2 {
			err = ErrHttpMalformedHeader
			return
		}
		headers[strings.ToLower(s[0])] = s[1]

		if len(headers) > MaxHeadersSupported {
			err = ErrExceedingHeaderCount
			return
		}
	}
	return
}

func ParseHttpRequest(r io.Reader) (ret HttpRequest, remainingBytes []byte, err error) {
	ret = HttpRequest{}

	bufReader := bufio.NewReader(r)
	ret.Headers, err = parseHttp(bufReader, func(s string) error {
		ss := strings.Split(s, " ")
		if len(ss) != 3 {
			return ErrHttpMalformedHeader
		}

		ret.Method = ss[0]
		ret.Url = ss[1]
		ret.Version = ss[2]

		if len(ret.Method) == 0 || len(ret.Url) == 0 || !strings.HasPrefix(ret.Version, "HTTP/") {
			return ErrHttpMalformedHeader
		}
		return nil
	})

	l, n, buffered := 0, 0, bufReader.Buffered()
	remainingBytes = make([]byte, buffered)

	for {
		if l >= buffered {
			break
		}

		n, err = bufReader.Read(remainingBytes[l:])
		if err != nil {
			return
		}
		l += n

	}

	return
}

var okResponse []byte = []byte("HTTP/1.1 200 OK\r\n\r\n")

func Copy(dst io.Writer, src io.Reader, signal chan error, initialData ...[]byte) {
	for _, d := range initialData {
		if len(d) == 0 {
			continue
		}

		if _, err := dst.Write(d); err != nil {
			signal <- err
			return
		}
	}

	if _, err := io.Copy(dst, src); err != nil {
		signal <- err
		return
	}

	close(signal)
}

func CopyFromRaw(dst *brotli.Writer, src *lib.Socket, signal chan error) {
	buf := make([]byte, 32*1024)
	blockRead := false
	writtenSinceFlush := 0

	for {
		var nr int
		var er error

		if blockRead {
			nr, er = src.Read(buf)
		} else {
			nr, er = src.TryRead(buf)
		}

		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])

			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					signal <- io.ErrShortWrite // long write
					return
				}
			}

			if ew != nil {
				signal <- ew
				return
			}
			if nr != nw {
				signal <- io.ErrShortWrite
				return
			}

			writtenSinceFlush += nw
		}

		if er != nil {
			if er != io.EOF {
				signal <- er
				return
			}

			close(signal)
			return
		}

		blockRead = nr == 0

		if nr == 0 && writtenSinceFlush > 0 {
			if err := dst.Flush(); err != nil {
				signal <- err
				return
			}
			writtenSinceFlush = 0
		}
	}
}

func Select[T any](chans []chan T) (index int, value T, ok bool) {
	rv := make([]reflect.SelectCase, len(chans))
	t := reflect.Value{}
	for i, v := range chans {
		rv[i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(v),
			Send: t,
		}
	}

	index, t, ok = reflect.Select(rv)
	if ok && !t.IsNil() {
		value = t.Interface().(T)
	}

	return
}

func Splice(conn net.Conn, cr *brotli.Reader, cw *brotli.Writer, addr string, dialer *net.Dialer, startCopy chan error, b []byte) {
	defer func() {
		defer recover()
		cw.Close()
	}()
	defer conn.Close()

	// TODO: make it configurable
	// TODO: check if the connection is alive
	dialContext, cancel := context.WithTimeout(context.Background(), time.Second*5)
	remote, err := dialer.DialContext(dialContext, "tcp", addr)
	cancel()

	if err != nil {
		return
	}

	defer remote.Close()
	raw, err := lib.NewSocket(remote.(*net.TCPConn))
	if err != nil {
		return
	}

	upstream := make(chan error)
	downstream := make(chan error)

	signals := []chan error{upstream, downstream}

	go Copy(remote, cr, upstream, b)

	if _, ok := <-startCopy; ok {
		return
	}

	go CopyFromRaw(cw, raw, downstream)

	for len(signals) > 0 {
		i, _, ok := Select(signals)

		if ok {
			signals = append(signals[:i], signals[i+1:]...)
			continue
		}

		return
	}
}

func HandleProxy(conn net.Conn, config *tls.Config, dialer *net.Dialer) {
	tlsConn := tls.Server(conn, config)

	cr := brotli.NewReader(tlsConn)
	req, b, err := ParseHttpRequest(cr)

	if err != nil {
		tlsConn.Close()
		return
	}

	cw := brotli.NewWriter(tlsConn)

	startCopy := make(chan error, 1)

	go Splice(tlsConn, cr, cw, req.Url, dialer, startCopy, b)
	if _, err := cw.Write(okResponse); err != nil {
		cw.Close()
		startCopy <- err
	}

	close(startCopy)

}
