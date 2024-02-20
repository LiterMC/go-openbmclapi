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
	"io"
	"sync/atomic"
	"time"

	"github.com/vbauerster/mpb/v8"
)

type ProxiedReader struct {
	io.Reader
	bar, total *mpb.Bar
	lastRead   time.Time
	lastInc    *atomic.Int64
}

func ProxyReader(r io.Reader, bar, total *mpb.Bar, lastInc *atomic.Int64) *ProxiedReader {
	return &ProxiedReader{
		Reader:  r,
		bar:     bar,
		total:   total,
		lastInc: lastInc,
	}
}

func (p *ProxiedReader) Read(buf []byte) (n int, err error) {
	start := p.lastRead
	if start.IsZero() {
		start = time.Now()
	}
	n, err = p.Reader.Read(buf)
	end := time.Now()
	p.lastRead = end
	used := end.Sub(start)

	p.bar.EwmaIncrBy(n, used)
	nowSt := end.UnixNano()
	last := p.lastInc.Swap(nowSt)
	p.total.EwmaIncrBy(n, (time.Duration)(nowSt-last)*time.Nanosecond)
	return
}

type ProxiedReadSeeker struct {
	io.ReadSeeker
	bar, total *mpb.Bar
	lastRead   time.Time
	lastInc    *atomic.Int64
}

func ProxyReadSeeker(r io.ReadSeeker, bar, total *mpb.Bar, lastInc *atomic.Int64) *ProxiedReadSeeker {
	return &ProxiedReadSeeker{
		ReadSeeker: r,
		bar:        bar,
		total:      total,
		lastInc:    lastInc,
	}
}

func (p *ProxiedReadSeeker) Read(buf []byte) (n int, err error) {
	start := p.lastRead
	if start.IsZero() {
		start = time.Now()
	}
	n, err = p.ReadSeeker.Read(buf)
	end := time.Now()
	p.lastRead = end
	used := end.Sub(start)

	p.bar.EwmaIncrBy(n, used)
	nowSt := end.UnixNano()
	last := p.lastInc.Swap(nowSt)
	p.total.EwmaIncrBy(n, (time.Duration)(nowSt-last)*time.Nanosecond)
	return
}
