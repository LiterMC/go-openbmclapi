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
	"bufio"
	"errors"
	"os"
	"sync"
)

var ErrNotFound = errors.New("Record not found")

// AddonDB is a type of database that always set/query data but rarely to add/remove them
// It's designed to save bmclapi path -> hash index
type AddonDB struct {
	mux sync.RWMutex
	fd  *os.File

	cache []path2HashRecord
}

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
		fd: fd,
	}
	return
}

func (db *AddonDB) searchRLocked(path string) (i int, exact bool) {
	i = sort.Search(db.cache, func(i int) bool {
		return db.cache[i].Path >= path
	})
	exact = db.cache[i].Path == path
	return
}

func (db *AddonDB) Get(path string) (hash string, err error) {
	db.mux.RLock()
	defer db.mux.RUnlock()

	i, exact := searchRLocked(path)
	if exact {
		return db.cache[i].Hash, nil
	}
	return "", ErrNotFound
}

func (db *AddonDB) Set(path string, hash string) (err error) {
	db.mux.Lock()
	defer db.mux.Unlock()

	i, exact := searchRLocked(path)
	if exact {
		db.cache[i].Hash = hash
	} else {
		db.cache = append(db.cache[:i+1], db.cache[i:]...)
		db.cache[i] = path2HashRecord{
			Path: path,
			hash: hash,
		}
	}
	// TOOD: save data
	return nil
}

func (db *AddonDB) Remove(path string) (err error) {
	db.mux.Lock()
	defer db.mux.Unlock()

	i, exact := searchRLocked(path)
	if !exact {
		return ErrNotFound
	}
	copy(db.cache[:i], db.cache[i+1:])
	db.cache = db.cache[:len(db.cache)-1]
	// TOOD: save data
	return nil
}

func (db *AddonDB) readRecordsLocked() (err error) {
	var buf [256]byte
	if _, err = db.fd.Seek(0, io.SeekStart); err != nil {
		return
	}
	br := bufio.NewReader(db.fd)
	if _, err = br.Read(buf[:4]); err != nil {
		return
	}
	buf[:4]
}

func (db *AddonDB) writeRecordsLocked() (err error) {
	var buf [256]byte
}
