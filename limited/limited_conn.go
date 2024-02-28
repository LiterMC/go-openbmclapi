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

package limited

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type RateController struct {
	*Semaphore
	readRate     int // bytes per second
	minReadRate  int
	writeRate    int // bytes per second
	minWriteRate int

	closed       atomic.Bool
	closeCh      chan struct{}
	rmux, wmux   sync.Mutex
	lastRead     time.Time
	preReadCount int
	readCount    int
	lastWrite    time.Time
	wroteCount   int
}

func NewRateController(maxConn int, readRate, writeRate int) *RateController {
	return &RateController{
		Semaphore:    NewSemaphore(maxConn),
		closeCh:      make(chan struct{}, 0),
		readRate:     readRate,
		minReadRate:  256,
		writeRate:    writeRate,
		minWriteRate: 256,
	}
}

func (l *RateController) ReadRate() int {
	return l.readRate
}

func (l *RateController) SetReadRate(rate int) {
	l.readRate = rate
	if rate > 0 && rate < l.minReadRate {
		l.minReadRate = rate
	}
}

func (l *RateController) MinReadRate() int {
	return l.minReadRate
}

func (l *RateController) SetMinReadRate(rate int) {
	l.minReadRate = rate
}

func (l *RateController) WriteRate() int {
	return l.writeRate
}

func (l *RateController) SetWriteRate(rate int) {
	l.writeRate = rate
	if rate > 0 && rate < l.minWriteRate {
		l.minWriteRate = rate
	}
}

func (l *RateController) MinWriteRate() int {
	return l.minWriteRate
}

func (l *RateController) SetMinWriteRate(rate int) {
	l.minWriteRate = rate
}

func (l *RateController) preRead(n int) int {
	if n <= 0 {
		return n
	}
	readRate := l.readRate
	if readRate <= 0 {
		return n
	}

	l.rmux.Lock()
	defer l.rmux.Unlock()

	avgRate := readRate
	if ln := l.Len(); ln > 0 {
		avgRate /= ln
	}
	if n > avgRate {
		n = avgRate
	}

	now := time.Now()
	diff := time.Second - now.Sub(l.lastRead)
	if diff <= 0 {
		l.lastRead = now
		l.preReadCount = 0
		l.readCount = 0
	}
	if l.preReadCount >= readRate {
		m := l.minReadRate
		if m > n {
			m = n
		}
		return m
	}
	l.preReadCount += n
	return n
}

func (l *RateController) afterRead(n int, less int) time.Duration {
	readRate := l.readRate
	if readRate <= 0 {
		return 0
	}
	if n < 0 {
		n = 0
	}

	l.rmux.Lock()
	defer l.rmux.Unlock()

	avgRate := readRate
	if ln := l.Len(); ln > 0 {
		avgRate /= ln
	}
	if n > avgRate {
		n = avgRate
	}

	now := time.Now()
	diff := time.Second - now.Sub(l.lastRead)
	if diff <= 0 {
		l.lastRead = now
		l.preReadCount = 0
		l.readCount = n
	} else {
		l.preReadCount -= less
		l.readCount += n
	}
	if n >= avgRate {
		// TODO: replace the magic number
		return time.Second
	}
	if l.readCount >= readRate {
		return diff
	}
	return 0
}

func (l *RateController) preWrite(n int) (int, time.Duration) {
	if n <= 0 {
		return n, 0
	}
	writeRate := l.writeRate
	if writeRate <= 0 {
		return n, 0
	}

	l.wmux.Lock()
	defer l.wmux.Unlock()

	now := time.Now()
	diff := time.Second - now.Sub(l.lastWrite)
	if diff <= 0 {
		l.lastWrite = now
		l.wroteCount = 0
	} else if l.wroteCount >= writeRate {
		m := l.minWriteRate
		if m > n {
			m = n
		}
		return m, diff
	}
	if n > writeRate {
		l.wroteCount += writeRate
		return writeRate, time.Second
	}
	l.wroteCount += n
	return n, 0
}

// Close will interrupted the incoming operations
// it will not close or interrupt the proxied connections and its operations
func (l *RateController) Close() error {
	if l.closed.CompareAndSwap(false, true) {
		close(l.closeCh)
	}
	return nil
}

func (l *RateController) DoWithContext(ctx context.Context, operator func() (net.Conn, error)) (net.Conn, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go func() {
		select {
		case <-ctx.Done():
		case <-l.closeCh:
			cancel(net.ErrClosed)
		}
	}()
	if !l.AcquireWithContext(ctx) {
		return nil, context.Cause(ctx)
	}
	c, err := operator()
	if err != nil {
		l.Release()
		return nil, err
	}
	conn := &LimitedConn{Conn: c, controller: l}
	return conn, nil
}

func (l *RateController) Do(operator func() (net.Conn, error)) (net.Conn, error) {
	if !l.AcquireWithNotify(l.closeCh) {
		return nil, net.ErrClosed
	}
	c, err := operator()
	if err != nil {
		l.Release()
		return nil, err
	}
	conn := &LimitedConn{Conn: c, controller: l}
	return conn, nil
}

func (l *RateController) DoReader(operator func() (io.Reader, error)) (io.ReadCloser, error) {
	if !l.AcquireWithNotify(l.closeCh) {
		return nil, net.ErrClosed
	}
	r, err := operator()
	if err != nil {
		l.Release()
		return nil, err
	}
	conn := &LimitedReader{Reader: r, controller: l}
	return conn, nil
}

func (l *RateController) DoWriter(operator func() (io.Writer, error)) (io.WriteCloser, error) {
	if !l.AcquireWithNotify(l.closeCh) {
		return nil, net.ErrClosed
	}
	w, err := operator()
	if err != nil {
		l.Release()
		return nil, err
	}
	conn := &LimitedWriter{Writer: w, controller: l}
	return conn, nil
}

type LimitedReader struct {
	io.Reader
	controller *RateController

	closed    atomic.Bool
	readAfter time.Time
}

var _ io.ReadCloser = (*LimitedReader)(nil)

func (r *LimitedReader) Read(buf []byte) (n int, err error) {
	if !r.readAfter.IsZero() {
		now := time.Now()
		if dur := r.readAfter.Sub(now); dur > 0 {
			time.Sleep(dur)
		}
	}
	m := r.controller.preRead(len(buf))
	n, err = r.Reader.Read(buf[:m])
	dur := r.controller.afterRead(n, m-n)
	if dur > 0 {
		r.readAfter = time.Now().Add(dur)
	} else {
		r.readAfter = time.Time{}
	}
	return
}

func (r *LimitedReader) Close() error {
	if r.closed.CompareAndSwap(false, true) {
		r.controller.Release()
	}
	if c, ok := r.Reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type LimitedWriter struct {
	io.Writer
	controller *RateController

	closed     atomic.Bool
	writeAfter time.Time
}

var _ io.WriteCloser = (*LimitedWriter)(nil)

func (w *LimitedWriter) Write(buf []byte) (n int, err error) {
	if len(buf) == 0 {
		return w.Writer.Write(buf)
	}
	var n0 int
	for n < len(buf) {
		if !w.writeAfter.IsZero() {
			now := time.Now()
			if dur := w.writeAfter.Sub(now); dur > 0 {
				time.Sleep(dur)
			}
		}
		m, dur := w.controller.preWrite(len(buf) - n)
		if dur > 0 {
			w.writeAfter = time.Now().Add(dur)
		} else {
			w.writeAfter = time.Time{}
		}
		if m > 0 {
			n0, err = w.Writer.Write(buf[n : n+m])
			n = n + n0
			if err != nil {
				return
			}
		}
	}
	return
}

func (w *LimitedWriter) Close() error {
	if w.closed.CompareAndSwap(false, true) {
		w.controller.Release()
	}
	if c, ok := w.Writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// LimitedConn is a net.Conn that proxied by RateController
// Concurrency on Read or Write is not allowed,
// but it is ok to Read and Write at same time.
type LimitedConn struct {
	net.Conn
	controller *RateController

	closed     atomic.Bool
	readAfter  time.Time
	writeAfter time.Time

	readDeadline  time.Time
	writeDeadline time.Time
}

var _ net.Conn = (*LimitedConn)(nil)

func (c *LimitedConn) Read(buf []byte) (n int, err error) {
	if !c.readAfter.IsZero() {
		now := time.Now()
		if dur := c.readAfter.Sub(now); dur > 0 {
			if !c.readDeadline.IsZero() {
				if deadDur := c.readDeadline.Sub(now); deadDur < dur {
					if deadDur > 0 {
						time.Sleep(deadDur)
					}
					err = os.ErrDeadlineExceeded
					return
				}
			}
			time.Sleep(dur)
		}
	}
	m := c.controller.preRead(len(buf))
	n, err = c.Conn.Read(buf[:m])
	dur := c.controller.afterRead(n, m-n)
	if dur > 0 {
		c.readAfter = time.Now().Add(dur)
	} else {
		c.readAfter = time.Time{}
	}
	return
}

func (c *LimitedConn) Write(buf []byte) (n int, err error) {
	if len(buf) == 0 {
		return c.Conn.Write(buf)
	}
	var n0 int
	for n < len(buf) {
		if !c.writeAfter.IsZero() {
			now := time.Now()
			if dur := c.writeAfter.Sub(now); dur > 0 {
				if !c.writeDeadline.IsZero() {
					if deadDur := c.writeDeadline.Sub(now); deadDur < dur {
						if deadDur > 0 {
							time.Sleep(deadDur)
						}
						err = os.ErrDeadlineExceeded
						return
					}
				}
				time.Sleep(dur)
			}
		}
		m, dur := c.controller.preWrite(len(buf) - n)
		if dur > 0 {
			c.writeAfter = time.Now().Add(dur)
		} else {
			c.writeAfter = time.Time{}
		}
		if m > 0 {
			n0, err = c.Conn.Write(buf[n : n+m])
			n = n + n0
			if err != nil {
				return
			}
		}
	}
	return
}

func (c *LimitedConn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		c.controller.Release()
	}
	return c.Conn.Close()
}

func (c *LimitedConn) SetDeadline(t time.Time) error {
	err1 := c.SetWriteDeadline(t)
	err2 := c.SetReadDeadline(t)
	if err1 != nil {
		return err1
	}
	return err2
}

func (c *LimitedConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return c.Conn.SetReadDeadline(t)
}

func (c *LimitedConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return c.Conn.SetWriteDeadline(t)
}

type LimitedListener struct {
	net.Listener

	*RateController
}

var _ net.Listener = (*LimitedListener)(nil)

func NewLimitedListener(l net.Listener, maxConn int, readRate, writeRate int) *LimitedListener {
	return &LimitedListener{
		Listener:       l,
		RateController: NewRateController(maxConn, readRate, writeRate),
	}
}

func (l *LimitedListener) Close() error {
	l.RateController.Close()
	return l.Listener.Close()
}

func (l *LimitedListener) Accept() (net.Conn, error) {
	return l.Do(l.Listener.Accept)
}

type NetDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type DialContextFn func(ctx context.Context, network, address string) (net.Conn, error)

var _ NetDialer = (DialContextFn)(nil)

func (f DialContextFn) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(ctx, network, address)
}

type LimitedDialer struct {
	Dialer NetDialer

	*RateController
}

var _ NetDialer = (*LimitedDialer)(nil)

func NewLimitedDialer(d NetDialer, maxConn int, readRate, writeRate int) *LimitedDialer {
	if d == nil {
		d = new(net.Dialer)
	}
	return &LimitedDialer{
		Dialer:         d,
		RateController: NewRateController(maxConn, readRate, writeRate),
	}
}

func (d *LimitedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.DoWithContext(ctx, func() (net.Conn, error) {
		return d.Dialer.DialContext(ctx, network, addr)
	})
}
