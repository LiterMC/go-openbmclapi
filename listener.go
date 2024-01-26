/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
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
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type LimitedListener struct {
	net.Listener

	slots        chan struct{}
	writeRate    int // bytes per second
	minWriteRate int

	mux        sync.Mutex
	closed     atomic.Bool
	closeCh    chan struct{}
	lastWrite  time.Time
	wroteCount int
}

var _ net.Listener = (*LimitedListener)(nil)

func NewLimitedListener(l net.Listener, maxConn int) *LimitedListener {
	return &LimitedListener{
		Listener:     l,
		slots:        make(chan struct{}, maxConn),
		minWriteRate: 256,
		closeCh:      make(chan struct{}, 0),
	}
}

func (l *LimitedListener) WriteRate() int {
	return l.writeRate
}

func (l *LimitedListener) SetWriteRate(rate int) {
	l.writeRate = rate
	if rate > 0 && rate < l.minWriteRate {
		l.minWriteRate = rate
	}
}

func (l *LimitedListener) MinWriteRate() int {
	return l.minWriteRate
}

func (l *LimitedListener) SetMinWriteRate(rate int) {
	l.minWriteRate = rate
}

func (l *LimitedListener) preWrite(n int) (int, time.Duration) {
	if n <= 0 {
		return n, 0
	}
	writeRate := l.writeRate
	if writeRate <= 0 {
		return n, 0
	}

	l.mux.Lock()
	defer l.mux.Unlock()

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

func (l *LimitedListener) Close() error {
	if l.closed.CompareAndSwap(false, true) {
		close(l.closeCh)
	}
	return l.Listener.Close()
}

func (l *LimitedListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
		c, err := l.Listener.Accept()
		if err != nil {
			<-l.slots
			return nil, err
		}
		conn := &LimitedConn{Conn: c, listener: l}
		return conn, nil
	case <-l.closeCh:
		return nil, net.ErrClosed
	}
}

type LimitedConn struct {
	net.Conn
	listener *LimitedListener

	closed     atomic.Bool
	writeAfter time.Time

	writeDeadline time.Time
}

var _ net.Conn = (*LimitedConn)(nil)

func (c *LimitedConn) Read(buf []byte) (n int, err error) {
	return c.Conn.Read(buf)
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
		m, dur := c.listener.preWrite(len(buf) - n)
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
		<-c.listener.slots
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

func (c *LimitedConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return c.Conn.SetWriteDeadline(t)
}
