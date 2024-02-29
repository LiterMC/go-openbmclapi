//go:build ignore

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

package database

import (
	"encoding/binary"
	"io"
	"os"
	"sort"
	"sync"
)

// AddonDB is a type of database that always set/query data but rarely to add/remove them
// It's designed to save bmclapi path -> hash index
type AddonDB struct {
	fdMux sync.Mutex
	cr    *chunkReader

	mux   sync.RWMutex
	cache []path2HashRecord
}

var _ DB = (*AddonDB)(nil)

// Each record will be saved like
// 1 byte valid flag; 1 byte length = N * chunks
// string : 2 bytes string length + string chunks in 256 bytes
// The record length must be able to multiply by 256
type path2HashRecord struct {
	Path string
	Hash string
}

func OpenAddonDB(path string) (db *AddonDB, err error) {
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_SYNC, 0644)
	if err != nil {
		return
	}
	db = &AddonDB{
		cr: newChunkReader(fd),
	}
	return
}

func (db *AddonDB) searchRLocked(path string) (i int, exact bool) {
	i = sort.Search(len(db.cache), func(i int) bool {
		return db.cache[i].Path >= path
	})
	exact = db.cache[i].Path == path
	return
}

func (db *AddonDB) Get(path string) (hash string, err error) {
	db.mux.RLock()
	defer db.mux.RUnlock()

	i, exact := db.searchRLocked(path)
	if exact {
		return db.cache[i].Hash, nil
	}
	return "", ErrNotFound
}

func (db *AddonDB) Set(path string, hash string) (err error) {
	db.mux.Lock()
	defer db.mux.Unlock()

	i, exact := db.searchRLocked(path)
	if exact {
		db.cache[i].Hash = hash
	} else {
		db.cache = append(db.cache[:i+1], db.cache[i:]...)
		db.cache[i] = path2HashRecord{
			Path: path,
			Hash: hash,
		}
	}
	// TOOD: save data
	return nil
}

func (db *AddonDB) Remove(path string) (err error) {
	db.mux.Lock()
	defer db.mux.Unlock()

	i, exact := db.searchRLocked(path)
	if !exact {
		return ErrNotFound
	}
	copy(db.cache[:i], db.cache[i+1:])
	db.cache = db.cache[:len(db.cache)-1]
	// TOOD: save data
	return nil
}

const chunkSize = 256

type chunkReader struct {
	buf  []byte
	r, w int

	fd   *os.File
	fpos int // fpos saves the current chunk position of fd
}

func newChunkReader(fd *os.File) *chunkReader {
	return &chunkReader{
		buf: make([]byte, chunkSize*8),
		fd:  fd,
	}
}

func (c *chunkReader) Seek(pos int) (err error) {
	if diff := c.fpos - pos; 0 < diff && diff*chunkSize < c.w {
		c.r = diff * chunkSize
		return nil
	}
	if _, err = c.fd.Seek((int64)(pos)*chunkSize, io.SeekStart); err != nil {
		return
	}
	c.fpos = pos
	c.r = 0
	c.w = 0
	return
}

func (c *chunkReader) NextChunk() (err error) {
	return c.NextNChunk(1)
}

func (c *chunkReader) NextNChunk(n int) (err error) {
	return c.Seek(c.fpos + n)
}

func (c *chunkReader) fill() (err error) {
	if c.r == c.w {
		c.r = 0
		c.w = 0
	}
	if len(c.buf) == 0 {
		panic("chunkReader: zero buffer")
	}
	if c.w == len(c.buf) {
		return
	}
	var n int
	n, err = c.fd.Read(c.buf[c.w:])
	c.w += n
	return
}

func (c *chunkReader) Read(buf []byte) (n int, err error) {
	if c.r == c.w {
		if err = c.fill(); err != nil {
			return
		}
	}
	n = copy(buf, c.buf[c.r:c.w])
	c.r += n
	return
}

func (db *AddonDB) readRecordsLocked() (err error) {
	db.fdMux.Lock()
	defer db.fdMux.Unlock()

	db.cache = nil

	cr := db.cr

	var buf [256]byte
	if err = cr.Seek(0); err != nil {
		return
	}
	if _, err = cr.Read(buf[:8]); err != nil {
		return
	}
	chunks := binary.BigEndian.Uint32(buf[0:4])
	sortedEnd := binary.BigEndian.Uint32(buf[4:8])
	if chunks == 0 {
		// TODO: maybe clear caches?
		return
	}
	if err = cr.NextChunk(); err != nil {
		return
	}

	// read a chunk group's header
	if _, err = cr.Read(buf[:]); err != nil {
		return
	}
	isValid := buf[0] == 1
	ckLen := binary.BigEndian.Uint16(buf[4:6])
	if isValid {
		str1Len := binary.BigEndian.Uint16(buf[8:10])
		str2Len := binary.BigEndian.Uint16(buf[10:12])
		(str1Len + 0xff) / 0x100
	}
}

func (db *AddonDB) writeRecords() (err error) {
	db.fdMux.Lock()
	defer db.fdMux.Unlock()
	return
}
