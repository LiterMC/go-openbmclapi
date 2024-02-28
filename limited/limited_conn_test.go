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

package limited

import (
	"testing"

	"io"
	"net"
	"sync"
)

type pipeListener struct {
	incoming  chan net.Conn
	closeOnce sync.Once
	closed    chan struct{}
}

var _ net.Listener = (*pipeListener)(nil)

func newPipeListener() *pipeListener {
	return &pipeListener{
		incoming: make(chan net.Conn, 0),
		closed:   make(chan struct{}, 0),
	}
}

func (l *pipeListener) Dial() (net.Conn, error) {
	remote, conn := net.Pipe()
	select {
	case l.incoming <- remote:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

type pipeListenerAddr struct{}

func (pipeListenerAddr) Network() string { return "imp" }
func (pipeListenerAddr) String() string  { return "<In Memory PipeListener>" }

func (l *pipeListener) Addr() net.Addr {
	return pipeListenerAddr{}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.incoming:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func TestLimitedListener(t *testing.T) {
	if true {
		return
	}

	imp := newPipeListener()
	l := NewLimitedListener(imp, 2, 0, 16)
	defer l.Close()

	l.SetMinWriteRate(6)

	const connCount = 5

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < connCount; i++ {
			conn, _ := imp.Dial()
			wg.Add(1)
			go func(i int, conn net.Conn) {
				defer wg.Done()
				defer conn.Close()
				t.Logf("Dialed: %d", i)
				var buf [256]byte
				for {
					n, err := conn.Read(buf[:])
					if err != nil {
						if err != io.EOF {
							t.Errorf("Read error: <%d> %v", i, err)
						}
						return
					}
					t.Logf("Read: <%d> %d %v", i, n, buf[:n])
				}
			}(i, conn)
		}
	}()

	for i := 0; i < connCount; i++ {
		conn, err := l.Accept()
		if err != nil {
			t.Errorf("Error when accepting: %v", err)
			break
		}
		t.Logf("Accepted: %d", i)
		wg.Add(1)
		go func(i int, conn net.Conn) {
			defer wg.Done()
			defer conn.Close()
			buf := make([]byte, 35)
			buf[0] = 1
			buf[15] = 2
			buf[17] = 3
			buf[34] = 4
			if i == 1 {
				buf = buf[12:24]
			} else if i == 4 {
				a, b := buf[0:14], buf[14:]
				n, err := conn.Write(a)
				t.Logf("Wrote: <%d> %v %v", i, n, err)
				n, err = conn.Write(b)
				t.Logf("Wrote: <%d> %v %v", i, n, err)
				return
			}
			n, err := conn.Write(buf)
			t.Logf("Wrote: <%d> %v %v", i, n, err)
		}(i, conn)
	}

	wg.Wait()
}
