package main

import (
	"crypto/tls"
	"crypto/x509"
	"lib"
	"lib/log"
	"net"
	"os"
	"time"
)

func main() {
	config := lib.LoadConfig[Config]()
	logger := lib.Must(log.NewLogger(nil))

	lib.AppScope.Init(logger)
	go logger.Start()
	defer logger.Close(true)

	serverAddr := lib.Must(lib.UrlToAddress(config.RemoteUrl))

	var rootCAs *x509.CertPool
	if config.RootCA != "" {
		rootCAs = x509.NewCertPool()
		cert := lib.Must(os.ReadFile(config.RootCA))
		rootCAs.AppendCertsFromPEM(cert)
	}

	certs := lib.Must(tls.LoadX509KeyPair(config.ClientCert, config.ClientKey))

	tlsConfig := tls.Config{
		ServerName:   serverAddr.Host,
		Certificates: []tls.Certificate{certs},
		RootCAs:      rootCAs,
		MinVersion:   tls.VersionTLS13,
		// resumption
		ClientSessionCache: tls.NewLRUClientSessionCache(1024),
	}

	lc := net.ListenConfig{}

	server := lib.Must(lc.Listen(lib.AppScope.Context, "tcp", config.ListenAddr))

	dialer := NewTFODialer()

	lib.AppScope.GoWithClose(func() {
		StartListener(lib.AppScope.Context, server, serverAddr.Address, &tlsConfig, dialer, logger)
	}, func() bool {
		server.(*net.TCPListener).SetDeadline(time.Now())
		return false
	})

	lib.AppScope.Done(false)
}
