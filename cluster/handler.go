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

package cluster

import (
	"net/http"

	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
)

func (cr *Cluster) HandleFile(req *http.Request, rw http.ResponseWriter, hash string, size int64) {
	defer log.RecoverPanic(nil)
	var err error
	if cr.storageManager.ForEachFromRandom(cr.storages, func(s storage.Storage) bool {
		opts := s.Options()
		log.Debugf("[handler]: Checking %s on storage %s ...", hash, opts.Id)

		sz, er := s.ServeDownload(rw, req, hash, size)
		if er != nil {
			log.Debugf("[handler]: File %s failed on storage %s: %v", hash, opts.Id, er)
			err = er
			return false
		}
		if sz >= 0 {
			cr.hits.Add(1)
			cr.hbts.Add(sz)
			cr.statManager.AddHit(sz, cr.ID(), opts.Id)
		}
		return true
	}) {
		return
	}
	http.Error(rw, err.Error(), http.StatusInternalServerError)
}
