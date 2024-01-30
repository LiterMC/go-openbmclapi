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
	"compress/gzip"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
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
	wrote  int64
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(buf []byte) (n int, err error) {
	n, err = w.ResponseWriter.Write(buf)
	w.wrote += (int64)(n)
	return
}

func (cr *Cluster) GetHandler() (handler http.Handler) {
	cr.handlerAPIv0 = http.StripPrefix("/api/v0", cr.initAPIv0())

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
			ua := req.Header.Get("User-Agent")
			srw := &statusResponseWriter{ResponseWriter: rw}
			start := time.Now()

			next.ServeHTTP(srw, req)

			used := time.Since(start)
			if config.RecordServeInfo {
				var addr string
				if config.TrustedXForwardedFor {
					// X-Forwarded-For: <client>, <proxy1>, <proxy2>
					adr, _ := split(req.Header.Get("X-Forwarded-For"), ',')
					addr = strings.TrimSpace(adr)
				}
				if addr == "" {
					addr, _, _ = net.SplitHostPort(req.RemoteAddr)
				}
				if used > time.Minute {
					used = used.Truncate(time.Second)
				} else if used > time.Second {
					used = used.Truncate(time.Microsecond)
				}
				logInfof("Serve %d | %12v | %7s | %-15s | %s | %-4s %s | %q",
					srw.status, used, bytesToUnit((float64)(srw.wrote)),
					addr, req.Proto,
					req.Method, req.RequestURI, ua)
			}
			if srw.status < 200 && 400 <= srw.status {
				return
			}
			if !strings.HasPrefix(req.URL.Path, "/download/") {
				return
			}
			var rec record
			rec.used = used.Seconds()
			rec.bytes = (float64)(srw.wrote)
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

					logInfof("Served %d requests, %s, used %.2fs, %s/s", total, bytesToUnit(totalBytes), totalUsed, bytesToUnit(totalBytes/60))
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
					case <-time.After(time.Minute * 10):
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
		if len(hash) < 4 {
			http.Error(rw, "404 Not Found", http.StatusNotFound)
			return
		}

		if _, ok := emptyHashes[hash]; ok {
			name := req.URL.Query().Get("name")
			rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
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

		// if use OSS redirect
		if cr.ossList != nil {
			cr.handleDownloadOSS(rw, req, hash)
			return
		}
		cr.handleDownload(rw, req, hash)
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
	acceptEncoding := splitCSV(req.Header.Get("Accept-Encoding"))
	name := req.URL.Query().Get("name")
	hashFilename := hashToFilename(hash)

	hasGzip := false
	isGzip := false
	path := filepath.Join(cr.cacheDir, hashFilename)
	if config.UseGzip {
		if _, err := os.Stat(path + ".gz"); err == nil {
			hasGzip = true
		}
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if !hasGzip {
			if hasGzip, err = cr.DownloadFile(req.Context(), hash); err != nil {
				http.Error(rw, "404 Status Not Found", http.StatusNotFound)
				return
			}
		}
		if hasGzip {
			isGzip = true
			path += ".gz"
		}
	}

	if !isGzip && rw.Header().Get("Range") != "" {
		fd, err := os.Open(path)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		defer fd.Close()
		counter := new(countReader)
		counter.ReadSeeker = fd

		rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		rw.Header().Set("X-Bmclapi-Hash", hash)
		http.ServeContent(rw, req, name, time.Time{}, counter)
		cr.hits.Add(1)
		cr.hbts.Add(counter.n)
		return
	}

	var r io.Reader
	if hasGzip && acceptEncoding["gzip"] != 0 {
		if !isGzip {
			isGzip = true
			path += ".gz"
		}
		fd, err := os.Open(path)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		defer fd.Close()
		r = fd
		rw.Header().Set("Content-Encoding", "gzip")
	} else {
		fd, err := os.Open(path)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		defer fd.Close()
		r = fd
		if isGzip {
			if r, err = gzip.NewReader(r); err != nil {
				logErrorf("Could not decompress %q: %v", path, err)
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			isGzip = false
		}
	}
	rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	rw.Header().Set("X-Bmclapi-Hash", hash)
	if !isGzip {
		if size, ok := cr.FileSet()[hash]; ok {
			rw.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		}
	} else if size, err := getFileSize(r); err == nil {
		rw.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	rw.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		var buf []byte
		{
			buf0 := bufPool.Get().(*[]byte)
			defer bufPool.Put(buf0)
			buf = *buf0
		}
		n, _ := io.CopyBuffer(rw, r, buf)
		cr.hits.Add(1)
		cr.hbts.Add(n)
	}
}

func (cr *Cluster) handleDownloadOSS(rw http.ResponseWriter, req *http.Request, hash string) {
	logDebug("[handler]: Preparing OSS redirect response")

	hashFilename := hashToFilename(hash)

	var err error
	// check if file was indexed in the fileset
	size, sizeCached := cr.FileSet()[hash]
	forEachSliceFromRandomIndex(len(cr.ossList), func(i int) bool {
		item := cr.ossList[i]
		logDebugf("[handler]: Checking file on OSS %d at %q ...", i, item.FolderPath)

		if !item.working.Load() {
			logDebugf("[handler]: OSS %d is not working", i)
			err = errors.New("All OSS server is down")
			return false
		}

		downloadDir := filepath.Join(item.FolderPath, "download")
		if !sizeCached {
			// check if the file exists
			path := filepath.Join(downloadDir, hashFilename)
			var stat os.FileInfo
			if stat, err = os.Stat(path); err != nil {
				logDebugf("[handler]: Cannot read file on OSS %d: %v", i, err)
				if errors.Is(err, os.ErrNotExist) {
					logInfof("[handler]: Downloading %s", hash)
					if e := cr.DownloadFileOSS(req.Context(), downloadDir, hash); e != nil {
						logInfof("[handler]: Could not downloaded %s\n\t%v", hash, e)
						return false
					}
					logInfof("[handler]: Downloaded %s", hash)
					if stat, err = os.Stat(path); err != nil {
						return false
					}
				} else {
					return false
				}
			}
			size = stat.Size()
		}

		var target string
		target, err = url.JoinPath(item.RedirectBase, "download", hashFilename)
		if err != nil {
			return false
		}
		if item.supportRange { // fix the size for Ranged request
			rg := req.Header.Get("Range")
			rgs, err := gosrc.ParseRange(rg, size)
			if err == nil && len(rgs) > 0 {
				var newSize int64 = 0
				for _, r := range rgs {
					newSize += r.Length
				}
				if newSize < size {
					size = newSize
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
}
