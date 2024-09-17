package lib

import (
	"context"
	"net"
)

type TcpOnServerError = func(error) bool
type TcpOnConnect = func(net.Conn)

type TcpServer struct {
	net.Listener
	OnServerError TcpOnServerError
	OnConnect     TcpOnConnect
}

func NewTcpServer(network string, address string) (*TcpServer, error) {
	listener, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	return &TcpServer{
		Listener: listener,
	}, nil
}

func (s *TcpServer) Start(ctx context.Context) {
	for !IsDone(ctx) {
		conn, err := s.Accept()
		if err != nil {
			if s.OnServerError != nil && s.OnServerError(err) {
				break
			}
		} else {
			go s.OnConnect(conn)
		}
	}
}

func TcpConnect(address string) (*net.TCPConn, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)

	if err != nil {
		return nil, err
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
