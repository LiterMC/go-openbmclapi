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
	"sync"
	"sync/atomic"
	"time"
)

type LimitedListener struct {
	net.Listener

	slots      chan struct{}
	closeOnce  sync.Once
	closed     chan struct{}
	writeLimit int // bytes per second
}

var _ net.Listener = (*LimitedListener)(nil)

func NewLimitedListener(l net.Listener, maxConn int) *LimitedListener {
	return &LimitedListener{
		Listener: l,
		slots:    make(chan struct{}, maxConn),
		closed:   make(chan struct{}, 0),
	}
}

func (l *LimitedListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
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
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

type LimitedConn struct {
	net.Conn
	listener *LimitedListener

	closed     atomic.Bool
	lastWrite  time.Time
	writeCount int
}

var _ net.Conn = (*LimitedConn)(nil)

func (c *LimitedConn) Read(buf []byte) (n int, err error) {
	return c.Conn.Read(buf)
}

func (c *LimitedConn) Write(buf []byte) (n int, err error) {
	writeLimit := c.listener.writeLimit
	if writeLimit <= 0 {
		return c.Conn.Write(buf)
	}
	now := time.Now()
	if c.lastWrite.IsZero() || c.lastWrite.Add(time.Second).Before(now) {
		c.lastWrite = now
	} else if c.writeCount >= writeLimit {
		time.Sleep(time.Second - now.Sub(c.lastWrite))
	}
	if len(buf) <= writeLimit {
		n, err = c.Conn.Write(buf)
		c.writeCount += n
	} else {
		var n0 int
		i, j := 0, 0
		for {
			j = i + writeLimit
			if j > len(buf) {
				j = len(buf)
			}
			n0, err = c.Conn.Write(buf[i:j])
			n += n0
			if err != nil {
				return
			}
			if i = j; i >= len(buf) {
				break
			}
			time.Sleep(time.Second)
		}
		c.lastWrite = time.Now()
		c.writeCount = n0
	}
	return
}

func (c *LimitedConn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		<-c.listener.slots
	}
	return c.Conn.Close()
}
