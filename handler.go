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
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/LiterMC/go-openbmclapi/internal/gosrc"
)

var zeroBuffer [1024 * 1024]byte

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
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (cr *Cluster) GetHandler() (handler http.Handler) {
	cr.handlerAPIv0 = http.StripPrefix("/api/v0", cr.initAPIv0())

	handler = cr
	{
		type record struct {
			used float64
			ua   string
		}
		recordCh := make(chan record, 1024)

		next := handler
		handler = (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
			ua := req.Header.Get("User-Agent")
			srw := &statusResponseWriter{ResponseWriter: rw}
			start := time.Now()

			next.ServeHTTP(srw, req)

			used := time.Since(start)
			if config.RecordServeInfo {
				addr, _, _ := net.SplitHostPort(req.RemoteAddr)
				if used > time.Minute {
					used = used.Truncate(time.Second)
				} else if used > time.Second {
					used = used.Truncate(time.Microsecond)
				}
				logInfof("Serve %d | %12v | %-15s | %s | %-4s %s | %q", srw.status, used, addr, req.Proto, req.Method, req.RequestURI, ua)
			}
			if srw.status < 200 && 400 <= srw.status {
				return
			}
			if !strings.HasPrefix(req.URL.Path, "/download/") {
				return
			}
			var rec record
			rec.used = used.Seconds()
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
				total     int64
				totalUsed float64
				uas       = make(map[string]int, 10)
			)
			for {
				select {
				case <-updateTicker.C:
					cr.stats.mux.Lock()
					total = 0
					totalUsed = 0
					for ua, v := range uas {
						if ua == "" {
							ua = "[Unknown]"
						}
						cr.stats.Accesses[ua] += v
					}
					clear(uas)
					cr.stats.mux.Unlock()
				case rec := <-recordCh:
					total++
					totalUsed += rec.used
					uas[rec.ua]++

					if total%100 == 0 {
						avg := (time.Duration)(totalUsed / (float64)(total) * (float64)(time.Second))
						logInfof("Served %d requests, total used %.2fs, avg %v", total, totalUsed, avg)
					}
				case <-disabled:
					return
				}
			}
		}()
	}
	return
}

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
		if len(hash) < 4 {
			http.Error(rw, "404 Status Not Found", http.StatusNotFound)
			return
		}
		name := req.Form.Get("name")

		hashFilename := hashToFilename(hash)

		// if use OSS redirect
		if cr.ossList != nil {
			logDebug("[handler]: Preparing OSS redirect response")
			var err error
			forEachSliceFromRandomIndex(len(cr.ossList), func(i int) bool {
				item := cr.ossList[i]
				logDebugf("[handler]: Checking OSS %d at %s ...", i, item.RedirectBase)

				if !item.working.Load() {
					logDebugf("[handler]: OSS %d is not working", i)
					err = errors.New("All OSS server is down")
					return false
				}

				// check if the file exists
				path := filepath.Join(item.FolderPath, hashFilename)
				var stat os.FileInfo
				if stat, err = os.Stat(path); err != nil {
					logDebugf("[handler]: File is not exists on OSS %d", i)
					if errors.Is(err, os.ErrNotExist) {
						if e := cr.DownloadFile(req.Context(), item.FolderPath, hash); e != nil {
							return false
						}
					} else {
						return false
					}
				}

				var target string
				target, err = url.JoinPath(item.RedirectBase, "download", hashFilename)
				if err != nil {
					return false
				}
				size := stat.Size()
				if item.supportRange { // fix the size for Ranged request
					rg := req.Header.Get("Range")
					rgs, err := gosrc.ParseRange(rg, size)
					if err == nil && len(rgs) > 0 {
						size = 0
						for _, r := range rgs {
							size += r.Length
						}
					}
				}
				http.Redirect(rw, req, target, http.StatusFound)
				cr.hits.Add(1)
				cr.hbts.Add(size)
				return true
			})
			if err != nil {
				logDebugf("[handler]: OSS redirect failed: %v", err)
				if errors.Is(err, os.ErrNotExist) {
					http.Error(rw, "404 Status Not Found", http.StatusNotFound)
					return
				}
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			logDebug("[handler]: OSS redirect successed")
			return
		}

		path := filepath.Join(cr.cacheDir, hashFilename)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := cr.DownloadFile(req.Context(), cr.cacheDir, hash); err != nil {
				http.Error(rw, "404 Status Not Found", http.StatusNotFound)
				return
			}
		}

		rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		fd, err := os.Open(path)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		defer fd.Close()
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Header().Set("X-Bmclapi-Hash", hash)
		counter := &countReader{ReadSeeker: fd}
		http.ServeContent(rw, req, name, time.Time{}, counter)
		cr.hits.Add(1)
		cr.hbts.Add(counter.n)
		return
	case strings.HasPrefix(rawpath, "/measure/"):
		if req.Header.Get("x-openbmclapi-secret") != cr.password {
			rw.WriteHeader(http.StatusForbidden)
			return
		}
		if method != http.MethodGet && method != http.MethodHead {
			rw.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if cr.ossList != nil {
			item := cr.ossList[0]
			target, err := url.JoinPath(item.RedirectBase, rawpath)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(rw, req, target, http.StatusFound)
			return
		}
		n, e := strconv.Atoi(rawpath[len("/measure/"):])
		if e != nil || n < 0 || n > 200 {
			http.Error(rw, e.Error(), http.StatusBadRequest)
			return
		}
		rw.Header().Set("Content-Length", strconv.Itoa(n*len(zeroBuffer)))
		rw.WriteHeader(http.StatusOK)
		if method == http.MethodGet {
			for i := 0; i < n; i++ {
				rw.Write(zeroBuffer[:])
			}
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
		pth := rawpath[len("/dashboard/"):]
		cr.serveDashboard(rw, req, pth)
		return
	case rawpath == "/" || rawpath == "/dashboard":
		http.Redirect(rw, req, "/dashboard/", http.StatusFound)
		return
	}
	http.NotFound(rw, req)
}
