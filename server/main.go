package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"lib"
	"lib/journald_logger"
	"net"
	"os"
	"syscall"
	"time"
)

func main() {
	config := lib.LoadConfig[Config]()
	logger := lib.Must(journald_logger.NewLogger(nil))

	lib.AppScope.Init(logger)
	go logger.Start()
	defer logger.Close(true)

	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 23 - TCP_FASTOPEN
				// 1024 - queue length. allow 1024 previous connection TFO cookies
				syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, 23, 1024)
			})
		},
	}

	certs := lib.Must(tls.LoadX509KeyPair(config.ServerCert, config.ServerKey))

	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{certs},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	if config.RootCA != "" {
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(lib.Must(os.ReadFile(config.RootCA)))

		tlsConfig.ClientCAs = certPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	tl := lib.Must(lc.Listen(lib.AppScope.Context, "tcp", config.ListenAddr))

	dialer := net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 30 - TCP_FASTOPEN_CONNECT
				syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, 30, 1)
			})
		},
	}

	lib.AppScope.GoWithClose(func() {
		StartListener(lib.AppScope.Context, tl, &tlsConfig, &dialer)
	}, func() bool {
		tl.(*net.TCPListener).SetDeadline(time.Now())
		return false
	})

	lib.AppScope.Done(false)
}

func StartListener(ctx context.Context, listener net.Listener, tlsConfig *tls.Config, dialer *net.Dialer) error {
	for !lib.IsDone(ctx) {
		conn, err := listener.Accept()

		if err != nil {
			if _, ok := err.(*net.OpError); ok {
				continue
			}

			return err
		}
		go HandleProxy(conn, tlsConfig, dialer)
	}

	return nil
}
