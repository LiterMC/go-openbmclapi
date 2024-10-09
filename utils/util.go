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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SyncMap[K comparable, V any] struct {
	l sync.RWMutex
	m map[K]V
}

func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: make(map[K]V),
	}
}

func (m *SyncMap[K, V]) Len() int {
	m.l.RLock()
	defer m.l.RUnlock()
	return len(m.m)
}

func (m *SyncMap[K, V]) RawMap() map[K]V {
	return m.m
}

func (m *SyncMap[K, V]) Clear() {
	m.l.Lock()
	defer m.l.Unlock()
	clear(m.m)
}

func (m *SyncMap[K, V]) Set(k K, v V) {
	m.l.Lock()
	defer m.l.Unlock()
	m.m[k] = v
}

func (m *SyncMap[K, V]) Get(k K) V {
	m.l.RLock()
	defer m.l.RUnlock()
	return m.m[k]
}

func (m *SyncMap[K, V]) Contains(k K) bool {
	m.l.RLock()
	defer m.l.RUnlock()
	_, ok := m.m[k]
	return ok
}

func (m *SyncMap[K, V]) GetOrSet(k K, setter func() V) (v V, had bool) {
	m.l.RLock()
	v, had = m.m[k]
	m.l.RUnlock()
	if had {
		return
	}
	m.l.Lock()
	defer m.l.Unlock()
	v, had = m.m[k]
	if !had {
		v = setter()
		m.m[k] = v
	}
	return
}

type Set[T comparable] map[T]struct{}

func NewSet[T comparable]() Set[T] {
	return make(Set[T])
}

func (s Set[T]) Clear() {
	clear(s)
}

func (s Set[T]) Put(v T) {
	s[v] = struct{}{}
}

func (s Set[T]) Contains(v T) bool {
	_, ok := s[v]
	return ok
}

func (s Set[T]) Remove(v T) bool {
	_, ok := s[v]
	if ok {
		delete(s, v)
	}
	return ok
}

func (s Set[T]) ToSlice(arr []T) []T {
	for v, _ := range s {
		arr = append(arr, v)
	}
	return arr
}

func (s Set[T]) String() string {
	var b strings.Builder
	b.WriteString("Set{")
	first := true
	for v := range s {
		if first {
			first = false
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%v", v)
	}
	b.WriteByte('}')
	return b.String()
}

func WalkCacheDir(cacheDir string, walker func(hash string, size int64) (err error)) (err error) {
	for _, dir := range Hex256 {
		files, err := os.ReadDir(filepath.Join(cacheDir, dir))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		for _, f := range files {
			if !f.IsDir() {
				if hash := f.Name(); len(hash) >= 2 && hash[:2] == dir {
					if info, err := f.Info(); err == nil {
						if err := walker(hash, info.Size()); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}
