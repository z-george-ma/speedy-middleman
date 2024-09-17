package lib

import (
	"errors"
	"net"
	"net/url"
	"runtime"
	"syscall"
)

type Socket struct {
	conn net.Conn
	sc   syscall.RawConn
	fd   syscall.Handle
}

func NewSocket(conn net.Conn) (*Socket, error) {
	var (
		sc  syscall.RawConn
		err error
	)
	switch conn.(type) {
	case *net.UnixConn:
		sc, err = conn.(*net.UnixConn).SyscallConn()
	case *net.TCPConn:
		sc, err = conn.(*net.TCPConn).SyscallConn()
	case *net.UDPConn:
		sc, err = conn.(*net.UDPConn).SyscallConn()
	case *net.IPConn:
		sc, err = conn.(*net.IPConn).SyscallConn()
	default:
		return nil, errors.New("unsupported connection type")
	}

	if err != nil {
		return nil, err
	}

	var fd syscall.Handle
	if err := sc.Control(func(f uintptr) {
		fd = syscall.Handle(f)
	}); err != nil {
		return nil, err
	}

	return &Socket{
		conn: conn,
		sc:   sc,
		fd:   fd,
	}, nil
}

func (s *Socket) TryRead(buf []byte) (n int, err error) {
	// broken on windows
	for {
		n, err = syscall.Read(s.fd, buf)
		if err == syscall.EINTR {
			continue
		}

		if err == syscall.EAGAIN {
			return 0, nil
		}

		return n, err
	}
}

func (s *Socket) Read(buf []byte) (n int, err error) {
	return s.conn.Read(buf)
}

func (s *Socket) EnsureRead(buf []byte) (n int, err error) {
	// broken on windows

	for {
		n, err = s.conn.Read(buf)
		if err == syscall.EINTR || err == syscall.EAGAIN {
			runtime.Gosched()
			continue
		}

		return
	}
}

func (s *Socket) TryWrite(buf []byte) (n int, err error) {
	// broken on windows

	for {
		n, err = syscall.Write(s.fd, buf)
		if err == syscall.EINTR {
			continue
		}

		if err == syscall.EAGAIN {
			return 0, nil
		}

		return n, err
	}
}

func (s *Socket) Write(buf []byte) (n int, err error) {
	return s.conn.Write(buf)
}

func (s *Socket) EnsureWrite(buf []byte) (n int, err error) {
	// broken on windows

	for {
		n, err = s.conn.Write(buf)
		if err == syscall.EINTR || err == syscall.EAGAIN {
			runtime.Gosched()
			continue
		}

		return
	}
}

type NetworkAddress struct {
	Scheme  string
	Host    string
	Port    string
	Address string
}

func UrlToAddress(addr string) (na NetworkAddress, err error) {
	u, err := url.Parse(addr)

	if err != nil {
		return
	}

	host := u.Hostname()
	port := u.Port()
	formattedAddr := u.Host

	if port == "" {
		switch u.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}

		formattedAddr += ":" + port
	}

	na = NetworkAddress{
		Scheme:  u.Scheme,
		Host:    host,
		Port:    port,
		Address: formattedAddr,
	}

	return
}
