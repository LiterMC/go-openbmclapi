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

package storage

import (
	"github.com/LiterMC/go-openbmclapi/utils"
)

// Manager manages a list of storages
type Manager struct {
	Options     []StorageOption
	Storages    []Storage
	weights     []uint
	totalWeight uint
	totalWeightsCache utils.SyncMap[[]string, *weightCache]
}

func NewManager(opts []StorageOption, storages []Storage) (m *Manager) {
	m = new(Manager)
	m.Options = opts
	m.Storages = storages
	m.weights = make([]uint, len(opts))
	m.totalWeight = 0
	m.totalWeightsCache = utils.NewSyncMap[[]string, *weightCache]()
	for i, s := range opts {
		m.weights[i] = s.Weight
		m.totalWeight += s.Weight
	}
	return
}

type weightCache struct{
	weights []uint
	total uint
}

func (m *Manager) ForEachFromRandom(storages []int, cb func(s Storage) (done bool)) (done bool) {
	data, _ := m.totalWeightsCache.GetOrSet(storages, func() (c *weightCache) {
		c = new(weightCache)
		c.weights = make([]int, len(storages))
		for i, j := range storages {
			w := m.weights[j]
			c.weights[i] = w
			c.total += w
		}
		return
	})
	return forEachFromRandomIndexWithPossibility(data.weights, data.total, cb)
}

func forEachFromRandomIndex(leng int, cb func(i int) (done bool)) (done bool) {
	if leng <= 0 {
		return false
	}
	start := utils.RandIntn(leng)
	for i := start; i < leng; i++ {
		if cb(i) {
			return true
		}
	}
	for i := 0; i < start; i++ {
		if cb(i) {
			return true
		}
	}
	return false
}

func forEachFromRandomIndexWithPossibility(poss []uint, total uint, cb func(i int) (done bool)) (done bool) {
	leng := len(poss)
	if leng == 0 {
		return false
	}
	if total == 0 {
		return forEachFromRandomIndex(leng, cb)
	}
	n := (uint)(utils.RandIntn((int)(total)))
	start := 0
	for i, p := range poss {
		if n < p {
			start = i
			break
		}
		n -= p
	}
	for i := start; i < leng; i++ {
		if cb(i) {
			return true
		}
	}
	for i := 0; i < start; i++ {
		if cb(i) {
			return true
		}
	}
	return false
}
