package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"

	"lib"
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

func Copy(dst io.Writer, src io.Reader, signal chan error, skipBytes int) {
	n := 0
	if skipBytes > n {
		b := make([]byte, skipBytes)
		r, err := src.Read(b)

		if err != nil {
			signal <- err
			return
		}

		n += r
	}

	if _, err := io.Copy(dst, src); err != nil {
		signal <- err
		return
	}

	close(signal)
}

var bufPool = sync.Pool{
	New: func() any {
		return make([]byte, 32*1024)
	},
}

var cwp = sync.Pool{
	New: func() any {
		return brotli.NewWriter(nil)
	},
}

var crp = sync.Pool{
	New: func() any {
		return brotli.NewReader(nil)
	},
}

func CopyFromRaw(dst *brotli.Writer, src *lib.Socket, signal chan error, initialData ...[]byte) {
	buf := bufPool.Get().([]byte)
	blockRead := false
	writtenSinceFlush := 0

	for _, d := range initialData {
		if len(d) == 0 {
			continue
		}

		n, err := dst.Write(d)
		if err != nil {
			bufPool.Put(buf)
			signal <- err
			return
		}

		writtenSinceFlush += n
	}

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
					bufPool.Put(buf)
					signal <- io.ErrShortWrite // long write
					return
				}
			}

			if ew != nil {
				bufPool.Put(buf)
				signal <- ew
				return
			}
			if nr != nw {
				bufPool.Put(buf)
				signal <- io.ErrShortWrite
				return
			}

			writtenSinceFlush += nw
		}

		if er != nil {
			bufPool.Put(buf)

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
				bufPool.Put(buf)
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

func CopyToRemote(conn *net.TCPConn, addr string, tlsConfig *tls.Config, dialer *net.Dialer, log lib.Logger, startCopy chan error, b ...[]byte) {
	defer conn.Close()

	raw, err := lib.NewSocket(conn)

	if err != nil {
		log.Err().Value("error", err.Error()).Msg("failed to create raw socket")
		return
	}

	remote, err := dialer.Dial("tcp", addr)

	if err != nil {
		log.Err().Value("error", err.Error()).Msg("failed to connect to peer")
		return
	}

	tlsConn := tls.Client(remote, tlsConfig)
	defer tlsConn.Close()

	if err := tlsConn.Handshake(); err != nil {
		log.Err().Value("error", err.Error()).Msg("failed to handshake with peer")
		return
	}

	upstream := make(chan error)
	downstream := make(chan error)

	signals := []chan error{upstream, downstream}

	cw := cwp.Get().(*brotli.Writer)
	cw.Reset(tlsConn)
	defer func() {
		defer recover()
		cw.Close()
		cwp.Put(cw)
	}()

	go CopyFromRaw(cw, raw, upstream, b...)

	if _, ok := <-startCopy; ok {
		return
	}

	cr := crp.Get().(*brotli.Reader)
	if cr.Reset(tlsConn) != nil {
		crp.Put(cr)
		return
	}

	go Copy(conn, cr, downstream, len(okResponse))

	for len(signals) > 0 {
		i, e, ok := Select(signals)

		if e == nil {
			log.Info().Msg("closed read/write")
		} else {
			log.Err().Value("error", e.Error()).Msg("failed to read/write")
		}

		if ok {
			signals = append(signals[:i], signals[i+1:]...)
			continue
		}

		crp.Put(cr)
		return
	}
}

func HandleConnection(conn net.Conn, serverAddr string, tlsConfig *tls.Config, dialer *net.Dialer, logger lib.Logger) {
	req, b, err := ParseHttpRequest(conn)

	if err != nil {
		conn.Close()
		logger.Err().Value("error", err.Error()).Msg("parse http request error")
		return
	}

	host := req.Url

	initData := []byte{}
	if req.Method != "CONNECT" {
		u, err := url.Parse(req.Url)
		if err != nil {
			logger.Err().Value("method", req.Method).Value("url", req.Url).Value("error", err.Error()).Msg("invalid url")
			conn.Close()
			return
		}

		if u.Scheme != "http" {
			logger.Err().Value("method", req.Method).Value("url", req.Url).Msg("unsupported schema")
			conn.Close()
			return
		}

		host = u.Hostname() + ":"
		if p := u.Port(); p != "" {
			host += p
		} else {
			host += "80"
		}

		sb := strings.Builder{}
		lib.BuildString(&sb, req.Method, " ", req.Url, " ", req.Version, "\r\n")
		for k, v := range req.Headers {
			lib.BuildString(&sb, k, ": ", v, "\r\n")
		}
		lib.BuildString(&sb, "\r\n")
		initData = []byte(sb.String())
	}

	startCopy := make(chan error, 1)

	log := logger.With().Value("url", req.Url).Value("method", req.Method).Logger()
	log.Info().Msg("connecting")

	// connection pool
	go CopyToRemote(conn.(*net.TCPConn), serverAddr, tlsConfig, dialer, log, startCopy, []byte("CONNECT "+host+" HTTP/1.1\r\n\r\n"), initData, b)

	if req.Method != "CONNECT" {
		close(startCopy)
		return
	}

	if _, err := conn.Write(okResponse); err != nil {
		log.Info().Value("error", err.Error()).Msg("failed to write ok response")

		startCopy <- err
	} else {
		close(startCopy)
	}
}

func StartListener(ctx context.Context, listener net.Listener, serverAddr string, tlsConfig *tls.Config, dialer *net.Dialer, logger lib.Logger) error {
	defer listener.Close()

	for !lib.IsDone(ctx) {
		conn, err := listener.Accept()

		if err != nil {
			if _, ok := err.(*net.OpError); ok {
				continue
			}

			logger.Err().Caller(1).Value("error", err.Error()).Msg("stop listening")

			return err
		}

		go HandleConnection(conn, serverAddr, tlsConfig, dialer, logger)
	}

	return nil
}
