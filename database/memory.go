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
)

type MemoryDB struct {
	mux  sync.RWMutex
	data map[string]*Record
}

var _ DB = (*MemoryDB)(nil)

func NewMemoryDB() *MemoryDB {
	return &MemoryDB{
		data: make(map[string]*Record),
	}
}

func (m *MemoryDB) Get(path string) (*Record, error) {
	m.mux.RLock()
	defer m.mux.RUnlock()

	record, ok := m.data[path]
	if !ok {
		return nil, ErrNotFound
	}
	return record, nil
}

func (m *MemoryDB) Set(record Record) error {
	m.mux.Lock()
	defer m.mux.Unlock()

	old, ok := m.data[record.Path]
	if ok && *old == record {
		return nil
	}
	m.data[record.Path] = &record
	return nil
}

func (m *MemoryDB) Remove(path string) error {
	m.mux.Lock()
	defer m.mux.Unlock()

	if _, ok := m.data[path]; !ok {
		return ErrNotFound
	}
	delete(m.data, path)
	return nil
}

func (m *MemoryDB) ForEach(cb func(*Record) error) error {
	m.mux.RLock()
	defer m.mux.RUnlock()

	for _, v := range m.data {
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}
