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
	"errors"
	"io"
)

type CountReader struct {
	io.ReadSeeker
	N int64
}

func (r *CountReader) Read(buf []byte) (n int, err error) {
	n, err = r.ReadSeeker.Read(buf)
	r.N += (int64)(n)
	return
}

type emptyReader struct{}

var (
	EmptyReader = emptyReader{}

	_ io.ReaderAt = EmptyReader
)

func (emptyReader) ReadAt(buf []byte, _ int64) (int, error) { return len(buf), nil }

type devNull struct{}

var (
	DevNull = devNull{}

	_ io.ReaderAt   = DevNull
	_ io.ReadSeeker = DevNull
	_ io.Writer     = DevNull
)

func (devNull) Read([]byte) (int, error)          { return 0, io.EOF }
func (devNull) ReadAt([]byte, int64) (int, error) { return 0, io.EOF }
func (devNull) Seek(int64, int) (int64, error)    { return 0, nil }
func (devNull) Write(buf []byte) (int, error)     { return len(buf), nil }

type NoLastNewLineWriter struct {
	io.Writer
}

var _ io.Writer = (*NoLastNewLineWriter)(nil)

func (w *NoLastNewLineWriter) Write(buf []byte) (int, error) {
	if l := len(buf) - 1; l >= 0 && buf[l] == '\n' {
		buf = buf[:l]
	}
	return w.Writer.Write(buf)
}

var ErrNotSeeker = errors.New("r is not an io.Seeker")

func GetReaderRemainSize(r io.Reader) (n int64, err error) {
	// for strings.Reader and bytes.Reader
	if l, ok := r.(interface{ Len() int }); ok {
		return (int64)(l.Len()), nil
	}
	if s, ok := r.(io.Seeker); ok {
		var cur, end int64
		if cur, err = s.Seek(0, io.SeekCurrent); err != nil {
			return
		}
		if end, err = s.Seek(0, io.SeekEnd); err != nil {
			return
		}
		if n = end - cur; n < 0 {
			n = 0
		}
		if _, err = s.Seek(cur, io.SeekStart); err != nil {
			return
		}
	} else {
		err = ErrNotSeeker
	}
	return
}
