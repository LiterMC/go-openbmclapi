/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2024 Kevin Z <zyxkad@gmail.com>
 * All rights reserved
 *
 *  This program is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU Affero General Public License as published
 *  by the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  This program is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU Affero General Public License for more details.
 *
 *  You should have received a copy of the GNU Affero General Public License
 *  along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// httpTLSListener will serve a http or a tls connection
// When Accept was called, if a pure http request is received,
// it will response and redirect the client to the https protocol.
// Else it will just return the tls connection
type httpTLSListener struct {
	net.Listener
	TLSConfig *tls.Config
	mux       sync.RWMutex
	hosts     []string
	port      string

	accepting  atomic.Bool
	acceptedCh chan net.Conn
	errCh      chan error
}

var _ net.Listener = (*httpTLSListener)(nil)

func newHttpTLSListener(l net.Listener, cfg *tls.Config, publicHosts []string, port uint16) net.Listener {
	return &httpTLSListener{
		Listener:   l,
		TLSConfig:  cfg,
		hosts:      publicHosts,
		port:       strconv.Itoa((int)(port)),
		acceptedCh: make(chan net.Conn, 1),
		errCh:      make(chan error, 1),
	}
}

func (s *httpTLSListener) Close() (err error) {
	err = s.Listener.Close()
	select {
	case conn := <-s.acceptedCh:
		conn.Close()
	default:
	}
	select {
	case <-s.errCh:
	default:
	}
	return
}

func (s *httpTLSListener) SetPublicPort(port string) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.port = port
}

func (s *httpTLSListener) getPublicPort() string {
	s.mux.RLock()
	defer s.mux.RUnlock()
	return s.port
}

func (s *httpTLSListener) maybeHTTPConn(c *connHeadReader) (ishttp bool) {
	if len(s.hosts) == 0 {
		return false
	}
	var buf [4096]byte
	i, n := 0, 0
READ_HEAD:
	for {
		m, err := c.ReadForHead(buf[i:])
		if err != nil {
			return false
		}
		n += m
		for ; i < n; i++ {
			b := buf[i]
			switch {
			case b == '\r': // first line of HTTP request end
				break READ_HEAD
			case b < 0x20 || 0x7e < b: // not in ascii printable range
				return false
			}
		}
	}
	// check if it's actually a HTTP request, not something else
	method, rest, _ := bytes.Cut(buf[:i], ([]byte)(" "))
	uurl, proto, _ := bytes.Cut(rest, ([]byte)(" "))
	if len(method) == 0 || len(uurl) == 0 || len(proto) == 0 {
		return false
	}
	_, _, ok := http.ParseHTTPVersion((string)(proto))
	if !ok {
		return false
	}
	_, err := url.ParseRequestURI((string)(uurl))
	if err != nil {
		return false
	}
	return true
}

func (s *httpTLSListener) accepter() {
	for s.accepting.CompareAndSwap(false, true) {
		conn, err := s.Listener.Accept()
		s.accepting.Store(false)
		if err != nil {
			s.errCh <- err
			return
		}
		go s.accepter()
		hr := &connHeadReader{Conn: conn}
		hr.SetReadDeadline(time.Now().Add(time.Second * 5))
		ishttp := s.maybeHTTPConn(hr)
		hr.SetReadDeadline(time.Time{})
		if !ishttp {
			// if it's not a http connection, it must be a tls connection
			s.acceptedCh <- tls.Server(hr, s.TLSConfig)
			return
		}
		go s.serveHTTP(hr)
	}
}

func (s *httpTLSListener) serveHTTP(conn net.Conn) {
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(time.Second * 15))
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})
	host, _, err := net.SplitHostPort(req.Host)
	if err != nil {
		host = req.Host
	}
	inhosts := false
	if host != "" {
		host = strings.ToLower(host)
		for _, h := range s.hosts {
			if h, ok := strings.CutPrefix(h, "*."); ok {
				if strings.HasSuffix(host, h) {
					inhosts = true
					break
				}
			} else if h == host {
				inhosts = true
				break
			}
		}
	}
	u := *req.URL
	u.Scheme = "https"
	if !inhosts {
		for _, h := range s.hosts {
			if !strings.HasSuffix(h, "*.") {
				host = h
				break
			}
		}
	}
	if host == "" {
		// we have nowhere to redirect
		return
	}
	u.Host = net.JoinHostPort(host, s.getPublicPort())
	resp := &http.Response{
		StatusCode: http.StatusPermanentRedirect,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
		Request:    req,
		Header: http.Header{
			"Location":     {u.String()},
			"X-Powered-By": {HeaderXPoweredBy},
		},
	}
	conn.SetWriteDeadline(time.Now().Add(time.Second * 15))
	resp.Write(conn)
}

func (s *httpTLSListener) Accept() (conn net.Conn, err error) {
	select {
	case conn = <-s.acceptedCh:
		return
	case err = <-s.errCh:
		return
	default:
	}
	go s.accepter()
	select {
	case conn = <-s.acceptedCh:
	case err = <-s.errCh:
	}
	return
}

// connHeadReader is used by httpTLSListener
// it wraps a net.Conn, and the first few bytes can be read multiple times
// the head buf will be discard when the main content starts to be read
type connHeadReader struct {
	net.Conn
	head     []byte
	headi    int
	headDone bool // the main content had start been read
}

func (c *connHeadReader) Head() []byte {
	return c.head
}

// ReadForHead will read the underlying net.Conn,
// and append the data to its internal head buffer
func (c *connHeadReader) ReadForHead(buf []byte) (n int, err error) {
	if c.headDone {
		panic("connHeadReader: Content is already started to read")
	}
	n, err = c.Conn.Read(buf)
	c.head = append(c.head, buf[:n]...)
	return
}

type connReaderForHead struct {
	c *connHeadReader
}

func (c *connReaderForHead) Read(buf []byte) (n int, err error) {
	return c.c.ReadForHead(buf)
}

func (c *connHeadReader) Read(buf []byte) (n int, err error) {
	if c.headi < len(c.head) {
		n = copy(buf, c.head[c.headi:])
		c.headi += n
		return
	}
	if !c.headDone {
		c.head = nil
		c.headDone = true
	}
	return c.Conn.Read(buf)
}
