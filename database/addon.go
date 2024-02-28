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

// AddonDB is a type of database that always add/set/query data but rarely to remove them
// It's designed to save bmclapi path -> hash index
type AddonDB struct {
	mux sync.RWMutex
	fd *os.File
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

func (db *AddonDB)Set(path string, hash string)(err error){
	//
}

func (db *AddonDB)decodeRecord()(err error){
	var buf [256]byte
	db.fd.Read(buf)
}
