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
	"sync"
)

const MbChunkSize = 1024 * 1024

var MbChunk [MbChunkSize]byte

const BUF_SIZE = 1024 * 512 // 512KB
var bufPool = sync.Pool{
	New: func() any {
		return new([]byte)
	},
}

func AllocBuf() ([]byte, func()) {
	suggestSize := BUF_SIZE
	ptr := bufPool.Get().(*[]byte)
	if *ptr == nil {
		*ptr = make([]byte, suggestSize)
	}
	return *ptr, func() {
		bufPool.Put(ptr)
	}
}
