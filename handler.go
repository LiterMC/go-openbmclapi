/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
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

package main

import (
	"bufio"
	"context"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type countReader struct {
	io.ReadSeeker
	n int64
}

func (r *countReader) Read(buf []byte) (n int, err error) {
	n, err = r.ReadSeeker.Read(buf)
	r.n += (int64)(n)
	return
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
	wrote  int64
}

var _ http.Hijacker = (*statusResponseWriter)(nil)

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(buf []byte) (n int, err error) {
	n, err = w.ResponseWriter.Write(buf)
	w.wrote += (int64)(n)
	return
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("ResponseWriter is not http.Hijacker")
}

const (
	AccessLogExtraCtxKey = "handle.access.extra"
)

func SetAccessInfo(req *http.Request, key string, value any) {
	if info, ok := req.Context().Value(AccessLogExtraCtxKey).(map[string]any); ok {
		info[key] = value
	}
}

type preAccessRecord struct {
	Type   string    `json:"type"`
	Time   time.Time `json:"time"`
	Addr   string    `json:"addr"`
	Method string    `json:"method"`
	URI    string    `json:"uri"`
	UA     string    `json:"ua"`
}

func (r *preAccessRecord) String() string {
	return fmt.Sprintf("Serving %s | %-4s %s | %q", r.Addr, r.Method, r.URI, r.UA)
}

type accessRecord struct {
	Type    string         `json:"type"`
	Status  int            `json:"status"`
	Used    time.Duration  `json:"used"`
	Content int64          `json:"content"`
	Addr    string         `json:"addr"`
	Proto   string         `json:"proto"`
	Method  string         `json:"method"`
	URI     string         `json:"uri"`
	UA      string         `json:"ua"`
	Extra   map[string]any `json:"extra,omitempty"`
}

func (r *accessRecord) String() string {
	used := r.Used
	if used > time.Minute {
		used = used.Truncate(time.Second)
	} else if used > time.Second {
		used = used.Truncate(time.Microsecond)
	}
	s := fmt.Sprintf("Serve %3d | %12v | %7s | %-15s | %s | %-4s %s | %q",
		r.Status, used, bytesToUnit((float64)(r.Content)),
		r.Addr, r.Proto,
		r.Method, r.URI, r.UA)
	if len(r.Extra) > 0 {
		buf, _ := json.Marshal(r.Extra)
		s += " | " + (string)(buf)
	}
	return s
}

func (cr *Cluster) GetHandler() (handler http.Handler) {
	cr.handlerAPIv0 = http.StripPrefix("/api/v0", cr.cliIdHandle(cr.initAPIv0()))

	handler = cr
	{
		type record struct {
			used  float64
			bytes float64
			ua    string
		}
		recordCh := make(chan record, 1024)

		next := handler
		handler = (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
			rw.Header().Set("X-Powered-By", "go-openbmclapi; url=https://github.com/LiterMC/go-openbmclapi")
			ua := req.Header.Get("User-Agent")
			var addr string
			if config.TrustedXForwardedFor {
				// X-Forwarded-For: <client>, <proxy1>, <proxy2>
				adr, _ := split(req.Header.Get("X-Forwarded-For"), ',')
				addr = strings.TrimSpace(adr)
			}
			if addr == "" {
				addr, _, _ = net.SplitHostPort(req.RemoteAddr)
			}
			srw := &statusResponseWriter{ResponseWriter: rw}
			start := time.Now()

			LogAccess(LogLevelDebug, &preAccessRecord{
				Type:   "pre-access",
				Time:   start,
				Addr:   addr,
				Method: req.Method,
				URI:    req.RequestURI,
				UA:     ua,
			})

			extraInfoMap := make(map[string]any)
			req = req.WithContext(context.WithValue(req.Context(), AccessLogExtraCtxKey, extraInfoMap))
			next.ServeHTTP(srw, req)

			used := time.Since(start)
			accRec := &accessRecord{
				Type:    "access",
				Status:  srw.status,
				Used:    used,
				Content: srw.wrote,
				Addr:    addr,
				Proto:   req.Proto,
				Method:  req.Method,
				URI:     req.RequestURI,
				UA:      ua,
				Extra:   extraInfoMap,
			}
			if len(extraInfoMap) > 0 {
				accRec.Extra = extraInfoMap
			}
			LogAccess(LogLevelInfo, accRec)

			if srw.status < 200 && 400 <= srw.status {
				return
			}
			if !strings.HasPrefix(req.URL.Path, "/download/") {
				return
			}
			var rec record
			rec.used = used.Seconds()
			rec.bytes = (float64)(srw.wrote)
			ua, _ = split(ua, ' ')
			rec.ua, _ = split(ua, '/')
			select {
			case recordCh <- rec:
			default:
			}
		})
		go func() {
			<-cr.WaitForEnable()
			disabled := cr.Disabled()

			updateTicker := time.NewTicker(time.Minute)
			defer updateTicker.Stop()

			var (
				total      int
				totalUsed  float64
				totalBytes float64
				uas        = make(map[string]int, 10)
			)
			for {
				select {
				case <-updateTicker.C:
					cr.stats.mux.Lock()

					logInfof("Served %d requests, total responsed body = %s, total used CPU time = %.2fs",
						total, bytesToUnit(totalBytes), totalUsed)
					for ua, v := range uas {
						if ua == "" {
							ua = "[Unknown]"
						}
						cr.stats.Accesses[ua] += v
					}

					total = 0
					totalUsed = 0
					totalBytes = 0
					clear(uas)

					cr.stats.mux.Unlock()
				case rec := <-recordCh:
					total++
					totalUsed += rec.used
					totalBytes += rec.bytes
					uas[rec.ua]++
				case <-disabled:
					total = 0
					totalUsed = 0
					totalBytes = 0
					clear(uas)

					select {
					case <-cr.WaitForEnable():
						disabled = cr.Disabled()
					case <-time.After(time.Hour):
						return
					}
				}
			}
		}()
	}
	return
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

func (cr *Cluster) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	method := req.Method
	u := req.URL

	rawpath := u.EscapedPath()
	switch {
	case strings.HasPrefix(rawpath, "/download/"):
		if method != http.MethodGet && method != http.MethodHead {
			rw.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		hash := rawpath[len("/download/"):]
		if !IsHex(hash) {
			http.Error(rw, hash+" is not a valid hash", http.StatusNotFound)
			return
		}

		query := req.URL.Query()
		if !checkQuerySign(hash, cr.clusterSecret, query) {
			http.Error(rw, "Cannot verify signature", http.StatusForbidden)
			return
		}

		logDebugf("Handling download %s", hash)
		cr.handleDownload(rw, req, hash)
		return
	case strings.HasPrefix(rawpath, "/measure/"):
		if method != http.MethodGet && method != http.MethodHead {
			rw.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		query := req.URL.Query()
		if !checkQuerySign(u.Path, cr.clusterSecret, query) {
			http.Error(rw, "Cannot verify signature", http.StatusForbidden)
			return
		}

		size := rawpath[len("/measure/"):]
		n, e := strconv.Atoi(size)
		if e != nil {
			http.Error(rw, e.Error(), http.StatusBadRequest)
			return
		} else if n < 0 || n > 200 {
			http.Error(rw, fmt.Sprintf("measure size %d out of range (0, 200]", n), http.StatusBadRequest)
			return
		}
		if err := cr.storages[0].ServeMeasure(rw, req, n); err != nil {
			logErrorf("Could not serve measure %d: %v", n, err)
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
		return
	case strings.HasPrefix(rawpath, "/api/"):
		version, _ := split(rawpath[len("/api/"):], '/')
		switch version {
		case "v0":
			cr.handlerAPIv0.ServeHTTP(rw, req)
			return
		}
	case strings.HasPrefix(rawpath, "/dashboard/"):
		if !config.Dashboard.Enable {
			http.NotFound(rw, req)
			return
		}
		pth := rawpath[len("/dashboard/"):]
		cr.serveDashboard(rw, req, pth)
		return
	case rawpath == "/" || rawpath == "/dashboard":
		http.Redirect(rw, req, "/dashboard/", http.StatusFound)
		return
	}
	http.NotFound(rw, req)
}

func (cr *Cluster) handleDownload(rw http.ResponseWriter, req *http.Request, hash string) {
	if _, ok := emptyHashes[hash]; ok {
		name := req.URL.Query().Get("name")
		rw.Header().Set("ETag", `"`+hash+`"`)
		rw.Header().Set("Cache-Control", "public,max-age=31536000,immutable") // cache for a year
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Header().Set("Content-Length", "0")
		if name != "" {
			rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		}
		rw.Header().Set("X-Bmclapi-Hash", hash)
		rw.WriteHeader(http.StatusOK)
		cr.hits.Add(1)
		// cr.hbts.Add(0) // no need to add zero
		return
	}

	if !cr.shouldEnable.Load() {
		// do not serve file if cluster is not enabled yet
		http.Error(rw, "Cluster is not enabled yet", http.StatusServiceUnavailable)
		return
	}

	var err error
	// check if file was indexed in the fileset
	size, ok := cr.CachedFileSize(hash)
	if !ok {
		if err := cr.DownloadFile(req.Context(), hash); err != nil {
			http.Error(rw, "404 not found", http.StatusNotFound)
			return
		}
	}
	var storage Storage
	forEachFromRandomIndexWithPossibility(cr.storageWeights, cr.storageTotalWeight, func(i int) bool {
		storage = cr.storages[i]
		logDebugf("[handler]: Checking file on Storage [%d] %s ...", i, storage.String())

		sz, er := storage.ServeDownload(rw, req, hash, size)
		if er != nil {
			err = er
			return false
		}
		if sz >= 0 {
			cr.hits.Add(1)
			cr.hbts.Add(sz)
		}
		return true
	})
	if storage != nil {
		SetAccessInfo(req, "storage", storage.String())
	}
	if err != nil {
		logDebugf("[handler]: failed to serve download: %v", err)
		if errors.Is(err, os.ErrNotExist) {
			http.Error(rw, "404 Status Not Found", http.StatusNotFound)
			return
		}
		if _, ok := err.(*HTTPStatusError); ok {
			http.Error(rw, err.Error(), http.StatusBadGateway)
		} else {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	logDebug("[handler]: download served successed")
}
