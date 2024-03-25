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
	"errors"
	"io"
	"net"
	"net/http"
	"path"
	"runtime"

	"github.com/LiterMC/go-openbmclapi/log"
)

type StatusResponseWriter struct {
	http.ResponseWriter
	Status int
	Wrote  int64
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

func (w *StatusResponseWriter) WriteHeader(status int) {
	if w.Status == 0 {
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

func NewHttpMiddleWareHandler(final http.Handler) *HttpMiddleWareHandler {
	return &HttpMiddleWareHandler{
		final: final,
	}
}

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

func (m *HttpMiddleWareHandler) Use(mids ...MiddleWare) {
	m.middles = append(m.middles, mids...)
}

func (m *HttpMiddleWareHandler) UseFunc(fns ...MiddleWareFunc) {
	for _, fn := range fns {
		m.middles = append(m.middles, fn)
	}
}
