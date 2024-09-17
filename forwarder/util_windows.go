package main

import (
	"net"
	"syscall"
)

func NewTFODialer() *net.Dialer {
	return &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 15 - TCP_FASTOPEN_CONNECT
				syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_TCP, 15, 1)
			})
		},
	}
}
