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

package main

import (
	"compress/gzip"
	"compress/zlib"
	"io"
)

type Compressor string

const (
	NullCompressor Compressor = ""
	ZlibCompressor Compressor = "zlib"
	GzipCompressor Compressor = "gzip"
)

func (c Compressor) Ext() string {
	switch c {
	case NullCompressor:
		return ""
	case ZlibCompressor:
		return ".zz"
	case GzipCompressor:
		return ".gz"
	default:
		panic("Unknown compressor: " + c)
	}
}

// Decompress the reader
func (c Compressor) WrapReader(r io.Reader) (io.Reader, error) {
	switch c {
	case NullCompressor:
		return r, nil
	case ZlibCompressor:
		return zlib.NewReader(r)
	case GzipCompressor:
		return gzip.NewReader(r)
	default:
		panic("Unknown compressor: " + c)
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}

// Compress the writer
func (c Compressor) WrapWriter(w io.Writer) io.WriteCloser {
	switch c {
	case NullCompressor:
		return nopWriteCloser{w}
	case ZlibCompressor:
		return zlib.NewWriter(w)
	case GzipCompressor:
		return gzip.NewWriter(w)
	default:
		panic("Unknown compressor: " + c)
	}
}
