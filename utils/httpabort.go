/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package utils

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
	"time"
)

// TCPReceiveBuffer is the SO_RCVBUF used on outbound sockets. A multi-MB default
// window lets a fast 404/error peer push an entire multi-MB body into the kernel
// before userspace closes — process RSS jumps ~body size with an empty Go heap
// profile. Keep this modest.
const TCPReceiveBuffer = 64 << 10

// TCPSendBuffer is the matching SO_SNDBUF for outbound sockets.
const TCPSendBuffer = 64 << 10

// ConnCapture holds the net.Conn used for a single HTTP round-trip (from
// httptrace.GotConn). One capture per in-flight request.
type ConnCapture struct {
	conn atomic.Value // net.Conn
}

// GotConn is an httptrace.GotConn hook.
func (c *ConnCapture) GotConn(info httptrace.GotConnInfo) {
	if info.Conn != nil {
		c.conn.Store(info.Conn)
	}
}

// Conn returns the captured connection, or nil.
func (c *ConnCapture) Conn() net.Conn {
	v := c.conn.Load()
	if v == nil {
		return nil
	}
	return v.(net.Conn)
}

// WithConnCapture attaches a ClientTrace that records the connection into cap.
func WithConnCapture(ctx context.Context, cap *ConnCapture) context.Context {
	if cap == nil {
		return ctx
	}
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: cap.GotConn,
	})
}

// LimitTCPBuffers sets modest SO_RCVBUF/SO_SNDBUF on a TCP connection so a
// remote cannot stuff multi-MB into this process before we abort a bad response.
func LimitTCPBuffers(c net.Conn) {
	tc := tcpConnOf(c)
	if tc == nil {
		return
	}
	_ = tc.SetReadBuffer(TCPReceiveBuffer)
	_ = tc.SetWriteBuffer(TCPSendBuffer)
}

// LimitedDialer returns a dialer that applies LimitTCPBuffers on every connect.
func LimitedDialer(timeout, keepAlive time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: keepAlive,
	}
}

// DialContextLimited dials and applies LimitTCPBuffers.
func DialContextLimited(d *net.Dialer, ctx context.Context, network, addr string) (net.Conn, error) {
	if d == nil {
		d = &net.Dialer{}
	}
	c, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	LimitTCPBuffers(c)
	return c, nil
}

// AbortHTTPResponse abandons a response without reading its body and forces a
// TCP RST on the underlying connection when possible.
func AbortHTTPResponse(resp *http.Response, conn net.Conn) {
	if tc := tcpConnOf(conn); tc != nil {
		_ = tc.SetLinger(0)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
		resp.Body = http.NoBody
	}
	ForceTCPAbort(conn)
}

// ForceTCPAbort issues a TCP RST (SetLinger 0) on the underlying TCP connection
// after unwrapping tls.Conn. Safe with nil or already-closed conns.
func ForceTCPAbort(c net.Conn) {
	if c == nil {
		return
	}
	tc := tcpConnOf(c)
	if tc != nil {
		_ = tc.SetLinger(0)
		_ = tc.Close()
		return
	}
	_ = c.Close()
}

func tcpConnOf(c net.Conn) *net.TCPConn {
	for c != nil {
		if tc, ok := c.(*net.TCPConn); ok {
			return tc
		}
		if tc, ok := c.(*tls.Conn); ok {
			c = tc.NetConn()
			continue
		}
		type netConner interface{ NetConn() net.Conn }
		if nc, ok := c.(netConner); ok {
			c = nc.NetConn()
			continue
		}
		return nil
	}
	return nil
}
