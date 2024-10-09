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

package utils

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/vbauerster/mpb/v8"
)

type pbReader struct {
	bar, total *mpb.Bar
	lastRead   time.Time
	lastInc    *atomic.Int64
}

func (p *pbReader) beforeRead() time.Time {
	start := p.lastRead
	if start.IsZero() {
		start = time.Now()
	}
	return start
}

func (p *pbReader) afterRead(n int, start time.Time) {
	end := time.Now()
	p.lastRead = end
	used := end.Sub(start)

	p.bar.EwmaIncrBy(n, used)
	nowSt := end.UnixNano()
	last := p.lastInc.Swap(nowSt)
	p.total.EwmaIncrBy(n, (time.Duration)(nowSt-last)*time.Nanosecond)
}

func (p *pbReader) read(r io.Reader, buf []byte) (n int, err error) {
	start := p.beforeRead()
	n, err = r.Read(buf)
	p.afterRead(n, start)
	return
}

type readerDeadline interface {
	SetReadDeadline(time.Time) error
}

func (p *pbReader) writeTo(r io.Reader, w io.Writer) (int64, error) {
	const maxChunkSize = 1024 * 16
	const maxUpdateInterval = time.Second
	lr := &io.LimitedReader{
		R: r,
		N: 0,
	}
	rd, deadOk := r.(readerDeadline)
	if deadOk {
		defer rd.SetReadDeadline(time.Time{})
	}
	var n int64
	for lr.N == 0 {
		lr.N = maxChunkSize
		start := p.beforeRead()
		if deadOk {
			rd.SetReadDeadline(time.Now().Add(maxUpdateInterval))
		}
		n0, err := io.Copy(w, lr)
		n += n0
		p.afterRead((int)(n0), start)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

type ProxiedPBReader struct {
	io.Reader
	pbr pbReader
}

func ProxyPBReader(r io.Reader, bar, total *mpb.Bar, lastInc *atomic.Int64) *ProxiedPBReader {
	return &ProxiedPBReader{
		Reader: r,
		pbr: pbReader{
			bar:     bar,
			total:   total,
			lastInc: lastInc,
		},
	}
}

func (p *ProxiedPBReader) Read(buf []byte) (int, error) {
	return p.pbr.read(p.Reader, buf)
}

func (p *ProxiedPBReader) WriteTo(w io.Writer) (int64, error) {
	return p.pbr.writeTo(p.Reader, w)
}

type ProxiedPBReadSeeker struct {
	io.ReadSeeker
	pbr pbReader
}

func ProxyPBReadSeeker(r io.ReadSeeker, bar, total *mpb.Bar, lastInc *atomic.Int64) *ProxiedPBReadSeeker {
	return &ProxiedPBReadSeeker{
		ReadSeeker: r,
		pbr: pbReader{
			bar:     bar,
			total:   total,
			lastInc: lastInc,
		},
	}
}

func (p *ProxiedPBReadSeeker) Read(buf []byte) (int, error) {
	return p.pbr.read(p.ReadSeeker, buf)
}

func (p *ProxiedPBReadSeeker) WriteTo(w io.Writer) (int64, error) {
	return p.pbr.writeTo(p.ReadSeeker, w)
}
