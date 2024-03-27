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
	"sync/atomic"
)

type slotInfo struct {
	id  int
	buf []byte
}

type BufSlots struct {
	c chan slotInfo
}

func NewBufSlots(size int) *BufSlots {
	c := make(chan slotInfo, size)
	for i := 0; i < size; i++ {
		c <- slotInfo{
			id:  i,
			buf: make([]byte, 1024*512),
		}
	}
	return &BufSlots{
		c: c,
	}
}

func (s *BufSlots) Len() int {
	return len(s.c)
}

func (s *BufSlots) Cap() int {
	return cap(s.c)
}

func (s *BufSlots) Alloc(ctx context.Context) (slotId int, buf []byte, free func()) {
	select {
	case slot := <-s.c:
		return slot.id, slot.buf, func() {
			s.c <- slot
		}
	case <-ctx.Done():
		return 0, nil, nil
	}
}

type Semaphore struct {
	c chan struct{}
}

// NewSemaphore create a semaphore
// zero or negative size means infinity space
func NewSemaphore(size int) *Semaphore {
	if size <= 0 {
		return nil
	}
	return &Semaphore{
		c: make(chan struct{}, size),
	}
}

func (s *Semaphore) Len() int {
	if s == nil {
		return 0
	}
	return len(s.c)
}

func (s *Semaphore) Cap() int {
	if s == nil {
		return 0
	}
	return cap(s.c)
}

func (s *Semaphore) Acquire() {
	if s == nil {
		return
	}
	s.c <- struct{}{}
}

func (s *Semaphore) AcquireWithContext(ctx context.Context) bool {
	if s == nil {
		return true
	}
	return s.AcquireWithNotify(ctx.Done())
}

func (s *Semaphore) AcquireWithNotify(notifier <-chan struct{}) bool {
	if s == nil {
		return true
	}
	select {
	case s.c <- struct{}{}:
		return true
	case <-notifier:
		return false
	}
}

func (s *Semaphore) Release() {
	if s == nil {
		return
	}
	<-s.c
}

func (s *Semaphore) Wait() {
	if s == nil {
		panic("Cannot wait on nil Semaphore")
	}
	for i := s.Cap(); i > 0; i-- {
		s.Acquire()
	}
}

func (s *Semaphore) WaitWithContext(ctx context.Context) bool {
	if s == nil {
		panic("Cannot wait on nil Semaphore")
	}
	for i := s.Cap(); i > 0; i-- {
		if !s.AcquireWithContext(ctx) {
			return false
		}
	}
	return true
}

type spProxyReader struct {
	io.Reader
	released atomic.Bool
	s        *Semaphore
}

func (r *spProxyReader) Close() error {
	if !r.released.Swap(true) {
		r.s.Release()
	}
	if c, ok := r.Reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (s *Semaphore) ProxyReader(r io.Reader) io.ReadCloser {
	return &spProxyReader{
		Reader: r,
		s:      s,
	}
}
