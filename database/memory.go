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
	"sync"
	"time"
)

type MemoryDB struct {
	fileRecMux  sync.RWMutex
	fileRecords map[string]*FileRecord

	tokenMux sync.RWMutex
	tokens   map[string]time.Time
}

var _ DB = (*MemoryDB)(nil)

func NewMemoryDB() *MemoryDB {
	return &MemoryDB{
		fileRecords: make(map[string]*FileRecord),
		tokens:      make(map[string]time.Time),
	}
}

func (m *MemoryDB) ValidJTI(jti string) (bool, error) {
	m.tokenMux.RLock()
	defer m.tokenMux.RUnlock()

	expire, ok := m.tokens[jti]
	if !ok {
		return false, ErrNotFound
	}
	if time.Now().After(expire) {
		return false, nil
	}
	return true, nil
}

func (m *MemoryDB) AddJTI(jti string, expire time.Time) error {
	m.tokenMux.Lock()
	defer m.tokenMux.Unlock()
	if _, ok := m.tokens[jti]; ok {
		return ErrExists
	}
	m.tokens[jti] = expire
	return nil
}

func (m *MemoryDB) RemoveJTI(jti string) error {
	m.tokenMux.RLock()
	_, ok := m.tokens[jti]
	m.tokenMux.RUnlock()
	if !ok {
		return ErrNotFound
	}

	m.tokenMux.Lock()
	defer m.tokenMux.Unlock()
	if _, ok := m.tokens[jti]; !ok {
		return ErrNotFound
	}
	delete(m.tokens, jti)
	return nil
}

func (m *MemoryDB) GetFileRecord(path string) (*FileRecord, error) {
	m.fileRecMux.RLock()
	defer m.fileRecMux.RUnlock()

	record, ok := m.fileRecords[path]
	if !ok {
		return nil, ErrNotFound
	}
	return record, nil
}

func (m *MemoryDB) SetFileRecord(record FileRecord) error {
	m.fileRecMux.Lock()
	defer m.fileRecMux.Unlock()

	old, ok := m.fileRecords[record.Path]
	if ok && *old == record {
		return nil
	}
	m.fileRecords[record.Path] = &record
	return nil
}

func (m *MemoryDB) RemoveFileRecord(path string) error {
	m.fileRecMux.Lock()
	defer m.fileRecMux.Unlock()

	if _, ok := m.fileRecords[path]; !ok {
		return ErrNotFound
	}
	delete(m.fileRecords, path)
	return nil
}

func (m *MemoryDB) ForEachFileRecord(cb func(*FileRecord) error) error {
	m.fileRecMux.RLock()
	defer m.fileRecMux.RUnlock()

	for _, v := range m.fileRecords {
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}
