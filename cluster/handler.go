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
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
)

func (cr *Cluster) HandleFile(req *http.Request, rw http.ResponseWriter, hash string, size int64) {
	defer log.RecoverPanic(nil)

	if !cr.Enabled() {
		// do not serve file if cluster is not enabled yet
		http.Error(rw, "Cluster is not enabled yet", http.StatusServiceUnavailable)
		return
	}

	if !cr.checkQuerySign(req, hash) {
		http.Error(rw, "Cannot verify signature", http.StatusForbidden)
		return
	}

	log.Debugf("Handling download %s", hash)

	keepaliveRec := req.Context().Value("go-openbmclapi.handler.no.record.for.keepalive") != true

	countUA := true
	if r := req.Header.Get("Range"); r != "" {
		api.SetAccessInfo(req, "range", r)
		if start, ok := parseRangeFirstStart(r); ok && start != 0 {
			countUA = false
		}
	}
	ua := ""
	if countUA {
		ua, _, _ = strings.Cut(req.UserAgent(), " ")
		ua, _, _ = strings.Cut(ua, "/")
	}

	rw.Header().Set("X-Bmclapi-Hash", hash)

	if _, ok := emptyHashes[hash]; ok {
		name := req.URL.Query().Get("name")
		rw.Header().Set("ETag", `"`+hash+`"`)
		rw.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // cache for a year
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Header().Set("Content-Length", "0")
		if name != "" {
			rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		}
		rw.WriteHeader(http.StatusOK)
		cr.statManager.AddHit(0, cr.ID(), "", ua)
		if keepaliveRec {
			cr.hits.Add(1)
		}
		return
	}

	api.SetAccessInfo(req, "cluster", cr.ID())

	var (
		sto storage.Storage
		err error
	)
	ok := cr.storageManager.ForEachFromRandom(cr.storages, func(s storage.Storage) bool {
		sto = s
		opts := s.Options()
		log.Debugf("[handler]: Checking %s on storage %s ...", hash, opts.Id)

		sz, er := s.ServeDownload(rw, req, hash, size)
		if er != nil {
			log.Debugf("[handler]: File %s failed on storage %s: %v", hash, opts.Id, er)
			err = er
			return false
		}
		if sz >= 0 {
			if keepaliveRec {
				cr.hits.Add(1)
				cr.hbts.Add(sz)
			}
			cr.statManager.AddHit(sz, cr.ID(), opts.Id, ua)
		}
		return true
	})
	if sto != nil {
		api.SetAccessInfo(req, "storage", sto.Id())
	}
	if ok {
		return
	}
	http.Error(rw, err.Error(), http.StatusInternalServerError)
}

func (cr *Cluster) HandleMeasure(req *http.Request, rw http.ResponseWriter, size int) {
	if !cr.Enabled() {
		// do not serve file if cluster is not enabled yet
		http.Error(rw, "Cluster is not enabled yet", http.StatusServiceUnavailable)
		return
	}

	if !cr.checkQuerySign(req, req.URL.Path) {
		http.Error(rw, "Cannot verify signature", http.StatusForbidden)
		return
	}

	api.SetAccessInfo(req, "cluster", cr.ID())
	storage := cr.storageManager.Storages[cr.storages[0]]
	api.SetAccessInfo(req, "storage", storage.Id())
	if err := storage.ServeMeasure(rw, req, size); err != nil {
		log.Errorf("Could not serve measure %d: %v", size, err)
		api.SetAccessInfo(req, "error", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
}

func (cr *Cluster) checkQuerySign(req *http.Request, hash string) bool {
	if cr.opts.SkipSignatureCheck {
		return true
	}
	query := req.URL.Query()
	sign, e := query.Get("s"), query.Get("e")
	if len(sign) == 0 || len(e) == 0 {
		return false
	}
	before, err := strconv.ParseInt(e, 36, 64)
	if err != nil {
		return false
	}
	if time.Now().UnixMilli() > before {
		return false
	}
	hs := crypto.SHA1.New()
	io.WriteString(hs, cr.Secret())
	io.WriteString(hs, hash)
	io.WriteString(hs, e)
	var (
		buf  [20]byte
		sbuf [27]byte
	)
	base64.RawURLEncoding.Encode(sbuf[:], hs.Sum(buf[:0]))
	if (string)(sbuf[:]) != sign {
		return false
	}
	return true
}

var emptyHashes = func() (hashes map[string]struct{}) {
	hashMethods := []crypto.Hash{
		crypto.MD5, crypto.SHA1,
	}
	hashes = make(map[string]struct{}, len(hashMethods))
	for _, h := range hashMethods {
		hs := hex.EncodeToString(h.New().Sum(nil))
		hashes[hs] = struct{}{}
	}
	return
}()

// Note: this method is a fast parse, it does not deeply check if the range is valid or not
func parseRangeFirstStart(rg string) (start int64, ok bool) {
	const b = "bytes="
	if rg, ok = strings.CutPrefix(rg, b); !ok {
		return
	}
	rg, _, _ = strings.Cut(rg, ",")
	if rg, _, ok = strings.Cut(rg, "-"); !ok {
		return
	}
	if rg = textproto.TrimString(rg); rg == "" {
		return -1, true
	}
	start, err := strconv.ParseInt(rg, 10, 64)
	if err != nil {
		return 0, false
	}
	return start, true
}
