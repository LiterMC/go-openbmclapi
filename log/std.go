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

package log

import (
	"bytes"
	"log"
)

type stdLogProxyWriter struct {
	buf []byte
	filters []func(line []byte) bool
}

var defaultStdLogProxyWriter = new(stdLogProxyWriter)

var ProxiedStdLog = log.New(defaultStdLogProxyWriter, "", 0)

// AddStdLogFilter add a filter before print std log
// If the filter returns true, the log will not be print
// It's designed to ignore http tls error
func AddStdLogFilter(filter func(line []byte) bool) {
	defaultStdLogProxyWriter.filters = append(defaultStdLogProxyWriter.filters, filter)
}

func (w *stdLogProxyWriter) log(buf []byte) {
	for _, f := range w.filters {
		if f(buf) {
			return
		}
	}
	Infof("[std]: %s", buf)
}

func (w *stdLogProxyWriter) Write(buf []byte) (n int, _ error) {
	n = len(buf) // always success
	if n == 0 {
		return
	}

	last := bytes.LastIndexByte(buf, '\n')
	if last < 0 {
		w.buf = append(w.buf, buf...)
		return
	}
	var splited [2][]byte
	lines := splitByteIgnoreLast(buf, '\n', splited[:0])
	w.buf = append(w.buf, lines[0]...)
	w.log(w.buf)
	for i := 1; i < len(lines); i++ {
		w.log(lines[i])
	}

	w.buf = append(w.buf[:0], buf[last + 1:]...)
	return
}

func splitByteIgnoreLast(buf []byte, sep byte, splited [][]byte) [][]byte {
	for i := 0; i < len(buf); i++ {
		if buf[i] == sep {
			splited = append(splited, buf[:i])
			buf = buf[i + 1:]
			i = 0
		}
	}
	return splited
}
