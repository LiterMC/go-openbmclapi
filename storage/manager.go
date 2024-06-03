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
	"errors"
	"net/http"
	"os"

	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type StorageManager struct {
	Options     []StorageOption
	Storages    []Storage
	weights     []uint
	totalWeight uint
}

func (m *StorageManager) Init(opts []StorageOption, storages []Storage) {
	m.Options = opts
	m.Storages = storages
	m.weights = make([]uint, len(opts))
	m.totalWeight = 0
	for i, s := range opts {
		m.weights[i] = s.Weight
		m.totalWeight += s.Weight
	}
}

func (m *StorageManager) GetStorageFlavor() string {
	typeCount := make(map[string]int, 2)
	for _, s := range m.Options {
		switch s.Type {
		case StorageLocal:
			typeCount["file"]++
		case StorageMount, StorageWebdav:
			typeCount["alist"]++
		default:
			log.Errorf("Unknown storage type %q", s.Type)
		}
	}
	flavor := ""
	for s, _ := range typeCount {
		if len(flavor) > 0 {
			flavor += "+"
		}
		flavor += s
	}
	return flavor
}

func (m *StorageManager) HandleMeasure(rw http.ResponseWriter, req *http.Request, size int) {
	if err := m.Storages[0].ServeMeasure(rw, req, size); err != nil {
		log.Errorf("Could not serve measure %d: %v", size, err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
}

func (m *StorageManager) HandleDownload(rw http.ResponseWriter, req *http.Request, hash string, size int64) (hits int32, bytes int64, storage string) {
	var (
		err error = ErrAllDown
		sto Storage
	)
	if utils.ForEachFromRandomIndexWithPossibility(m.weights, m.totalWeight, func(i int) bool {
		sto = m.Storages[i]
		log.Debugf("[handler]: Checking %s on storage [%d] %s ...", hash, i, sto.String())

		sz, er := sto.ServeDownload(rw, req, hash, size)
		if er != nil {
			log.Debugf("[handler]: File %s failed on storage [%d] %s: %v", hash, i, sto.String(), er)
			err = er
			return false
		}
		if sz >= 0 {
			opts := m.Options[i]
			hits, bytes, storage = 1, sz, opts.Id
		}
		return true
	}) {
		err = nil
	}
	if err == nil {
		return
	}
	log.Debugf("[handler]: failed to serve download: %v", err)
	if errors.Is(err, os.ErrNotExist) {
		http.Error(rw, "404 Status Not Found", http.StatusNotFound)
		return
	}
	if err == ErrNotWorking {
		err = ErrAllDown
	}
	if _, ok := err.(*utils.HTTPStatusError); ok {
		http.Error(rw, err.Error(), http.StatusBadGateway)
	} else {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
	return
}
