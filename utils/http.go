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

package utils

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/log"
)

type StatusResponseWriter struct {
	http.ResponseWriter
	Status            int
	Wrote             int64
	beforeWriteHeader []func(status int)
}

var _ http.Hijacker = (*StatusResponseWriter)(nil)

func WrapAsStatusResponseWriter(rw http.ResponseWriter) *StatusResponseWriter {
	if srw, ok := rw.(*StatusResponseWriter); ok {
		return srw
	}
	return &StatusResponseWriter{ResponseWriter: rw}
}

func getCaller() (caller runtime.Frame) {
	pc := make([]uintptr, 16)
	n := runtime.Callers(3, pc)
	frames := runtime.CallersFrames(pc[:n])
	frame, more := frames.Next()
	_ = more
	return frame
}

func (w *StatusResponseWriter) BeforeWriteHeader(cb func(status int)) {
	w.beforeWriteHeader = append(w.beforeWriteHeader, cb)
}

func (w *StatusResponseWriter) WriteHeader(status int) {
	if w.Status == 0 {
		for _, cb := range w.beforeWriteHeader {
			cb(status)
		}
		w.Status = status
		w.ResponseWriter.WriteHeader(status)
	} else {
		caller := getCaller()
		log.Warnf("http: superfluous response.WriteHeader call with status %d from %s (%s:%d)",
			status, caller.Function, path.Base(caller.File), caller.Line)
	}
}

func (w *StatusResponseWriter) Write(buf []byte) (n int, err error) {
	n, err = w.ResponseWriter.Write(buf)
	w.Wrote += (int64)(n)
	return
}

func (w *StatusResponseWriter) ReadFrom(r io.Reader) (n int64, err error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(r)
		w.Wrote += n
		return
	}
	n, err = io.Copy(w.ResponseWriter, r)
	w.Wrote += n
	return
}

func (w *StatusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("ResponseWriter is not http.Hijacker")
}

type MiddleWare interface {
	ServeMiddle(rw http.ResponseWriter, req *http.Request, next http.Handler)
}

type MiddleWareFunc func(rw http.ResponseWriter, req *http.Request, next http.Handler)

var _ MiddleWare = (MiddleWareFunc)(nil)

func (m MiddleWareFunc) ServeMiddle(rw http.ResponseWriter, req *http.Request, next http.Handler) {
	m(rw, req, next)
}

type HttpMiddleWareHandler struct {
	final   http.Handler
	middles []MiddleWare
}

var _ http.Handler = (*HttpMiddleWareHandler)(nil)

func NewHttpMiddleWareHandler(final http.Handler, middles ...MiddleWare) *HttpMiddleWareHandler {
	return &HttpMiddleWareHandler{
		final:   final,
		middles: middles,
	}
}

// Handler returns the final http.Handler
func (m *HttpMiddleWareHandler) Handler() http.Handler {
	return m.final
}

// ServeHTTP implements http.Handler
// It will invoke the middlewares in order
func (m *HttpMiddleWareHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	i := 0
	var getNext func() http.Handler
	getNext = func() http.Handler {
		j := i
		if j > len(m.middles) {
			// unreachable
			panic("HttpMiddleWareHandler: called getNext too much times")
		}
		i++
		if j == len(m.middles) {
			return m.final
		}
		mid := m.middles[j]

		called := false
		return (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
			if called {
				panic("HttpMiddleWareHandler: Called next function twice")
			}
			called = true
			mid.ServeMiddle(rw, req, getNext())
		})
	}
	getNext().ServeHTTP(rw, req)
}

// Use append MiddleWares to the middleware chain
func (m *HttpMiddleWareHandler) Use(mids ...MiddleWare) {
	m.middles = append(m.middles, mids...)
}

// UseFunc append MiddleWareFuncs to the middleware chain
func (m *HttpMiddleWareHandler) UseFunc(fns ...MiddleWareFunc) {
	for _, fn := range fns {
		m.middles = append(m.middles, fn)
	}
}

// HttpMethodHandler pass down http requests to different handler based on the request methods
// The HttpMethodHandler should not be modified after called ServeHTTP
type HttpMethodHandler struct {
	Get     http.Handler
	Head    bool
	Post    http.Handler
	Put     http.Handler
	Patch   http.Handler
	Delete  http.Handler
	Connect http.Handler
	Options http.Handler
	Trace   http.Handler

	allows     string
	allowsOnce sync.Once
}

var _ http.Handler = (*HttpMethodHandler)(nil)

// ServeHTTP implements http.Handler
// Once ServeHTTP is called the HttpMethodHandler should not be modified
func (m *HttpMethodHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodHead:
		if !m.Head {
			break
		}
		fallthrough
	case http.MethodGet:
		if m.Get != nil {
			m.Get.ServeHTTP(rw, req)
			return
		}
	case http.MethodPost:
		if m.Post != nil {
			m.Post.ServeHTTP(rw, req)
			return
		}
	case http.MethodPut:
		if m.Put != nil {
			m.Put.ServeHTTP(rw, req)
			return
		}
	case http.MethodPatch:
		if m.Patch != nil {
			m.Patch.ServeHTTP(rw, req)
			return
		}
	case http.MethodDelete:
		if m.Delete != nil {
			m.Delete.ServeHTTP(rw, req)
			return
		}
	case http.MethodConnect:
		if m.Connect != nil {
			m.Connect.ServeHTTP(rw, req)
			return
		}
	case http.MethodOptions:
		if m.Options != nil {
			m.Options.ServeHTTP(rw, req)
			return
		}
	case http.MethodTrace:
		if m.Trace != nil {
			m.Trace.ServeHTTP(rw, req)
			return
		}
	}
	m.allowsOnce.Do(func() {
		allows := make([]string, 0, 5)
		if m.Get != nil {
			allows = append(allows, http.MethodGet)
			if m.Head {
				allows = append(allows, http.MethodGet)
			}
		}
		if m.Post != nil {
			allows = append(allows, http.MethodPost)
		}
		if m.Put != nil {
			allows = append(allows, http.MethodPut)
		}
		if m.Patch != nil {
			allows = append(allows, http.MethodPatch)
		}
		if m.Delete != nil {
			allows = append(allows, http.MethodDelete)
		}
		if m.Connect != nil {
			allows = append(allows, http.MethodConnect)
		}
		if m.Options != nil {
			allows = append(allows, http.MethodOptions)
		}
		if m.Trace != nil {
			allows = append(allows, http.MethodTrace)
		}
		m.allows = strings.Join(allows, ", ")
	})
	rw.Header().Set("Allow", m.allows)
	rw.WriteHeader(http.StatusMethodNotAllowed)
}

// HTTPTLSListener will serve a http or a tls connection
// When Accept was called, if a pure http request is received,
// it will response and redirect the client to the https protocol.
// Else it will just return the tls connection
type HTTPTLSListener struct {
	net.Listener
	TLSConfig     atomic.Pointer[tls.Config]
	DoRedirect    bool
	AllowUnsecure bool

	accepting  atomic.Bool
	acceptedCh chan net.Conn
	errCh      chan error
}

var _ net.Listener = (*HTTPTLSListener)(nil)

func NewHttpTLSListener(l net.Listener, cfg *tls.Config) *HTTPTLSListener {
	h := &HTTPTLSListener{
		Listener:      l,
		DoRedirect:    true,
		AllowUnsecure: false,

		acceptedCh: make(chan net.Conn, 1),
		errCh:      make(chan error, 1),
	}
	h.TLSConfig.Store(cfg)
	return h
}

func (s *HTTPTLSListener) Close() (err error) {
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

func (s *HTTPTLSListener) maybeHTTPConn(c *connHeadReader) (ishttp bool) {
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

func (s *HTTPTLSListener) accepter() {
	for s.accepting.CompareAndSwap(false, true) {
		conn, err := s.Listener.Accept()
		s.accepting.Store(false)
		if err != nil {
			s.errCh <- err
			return
		}
		tlsCfg := s.TLSConfig.Load()
		if tlsCfg == nil {
			s.acceptedCh <- conn
			return
		}
		go s.accepter()
		hr := &connHeadReader{Conn: conn}
		hr.SetReadDeadline(time.Now().Add(time.Second * 5))
		ishttp := s.maybeHTTPConn(hr)
		hr.SetReadDeadline(time.Time{})
		if !ishttp {
			// if it's not a http connection, it must be a tls connection
			s.acceptedCh <- tls.Server(hr, tlsCfg)
			return
		}
		if s.AllowUnsecure {
			s.acceptedCh <- hr
			return
		}
		go s.serveHTTP(hr)
	}
}

func (s *HTTPTLSListener) serveHTTP(conn net.Conn) {
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(time.Second * 15))
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})
	// host, _, err := net.SplitHostPort(req.Host)
	// if err != nil {
	// 	host = req.Host
	// }
	u := *req.URL
	u.Scheme = "https"
	if !s.DoRedirect {
		body := strings.NewReader("Sent http request on https server")
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			ProtoMajor: req.ProtoMajor,
			ProtoMinor: req.ProtoMinor,
			Request:    req,
			Header: http.Header{
				"Content-Type": {"text/plain"},
				"X-Powered-By": {build.HeaderXPoweredBy},
			},
			ContentLength: (int64)(body.Len()),
		}
		conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
		resp.Write(conn)
		io.Copy(conn, body)
		return
	}
	// u.Host = net.JoinHostPort(host, s.GetPublicPort())
	resp := &http.Response{
		StatusCode: http.StatusPermanentRedirect,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
		Request:    req,
		Header: http.Header{
			"Location":     {u.String()},
			"X-Powered-By": {build.HeaderXPoweredBy},
		},
	}
	conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	resp.Write(conn)
}

func (s *HTTPTLSListener) Accept() (conn net.Conn, err error) {
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

// connHeadReader is used by HTTPTLSListener
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
