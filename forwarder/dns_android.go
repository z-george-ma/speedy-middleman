package main

import (
	"context"
	"net"
)

func SetDNS() {
	var dialer net.Dialer
	net.DefaultResolver = &net.Resolver{
		PreferGo: false,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp", "94.140.14.14:53") // adguard
		},
	}
}
