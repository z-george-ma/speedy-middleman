//go:build !windows

package main

import (
	"net"
	"syscall"
)

func NewTFODialer() *net.Dialer {
	return &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 30 - TCP_FASTOPEN_CONNECT
				syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, 30, 1)
			})
		},
	}
}
