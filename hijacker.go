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
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/utils"
)

func getDialerWithDNS(dnsaddr string) *net.Dialer {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return net.Dial(network, dnsaddr)
		},
	}
	return &net.Dialer{
		Resolver: resolver,
	}
}

type downloadHandlerFn = func(rw http.ResponseWriter, req *http.Request, hash string)

type HjProxy struct {
	client          *http.Client
	fileMap         database.DB
	downloadHandler downloadHandlerFn

	cacheMux  sync.RWMutex
	cache     map[string]*cacheStat
	saveTimer *time.Timer
}

func NewHjProxy(client *http.Client, fileMap database.DB, downloadHandler downloadHandlerFn) (h *HjProxy) {
	cli := new(http.Client)
	*cli = *client
	cli.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	h = &HjProxy{
		client:          cli,
		fileMap:         fileMap,
		downloadHandler: downloadHandler,
	}
	h.loadCache()
	return
}

func hjResponseWithCache(rw http.ResponseWriter, req *http.Request, c *cacheStat, force bool) (ok bool) {
	if c == nil {
		return false
	}
	cacheFileName := filepath.Join(config.Hijack.LocalCachePath, filepath.FromSlash(req.URL.Path))
	age := c.ExpiresAt - time.Now().Unix()
	if !force && age <= 0 {
		return false
	}
	stat, err := os.Stat(cacheFileName)
	if err != nil {
		return false
	}
	if stat.Size() != c.Size {
		return false
	}
	fd, err := os.Open(cacheFileName)
	if err != nil {
		return false
	}
	defer fd.Close()
	if age > 0 {
		rw.Header().Set("Cache-Control", "public, max-age="+strconv.FormatInt(age, 10))
	}
	http.ServeContent(rw, req, path.Base(req.URL.Path), stat.ModTime(), fd)
	return true
}

const hijackingHost = "bmclapi2.bangbang93.com"

func (h *HjProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !config.Hijack.Enable {
		http.Error(rw, "Hijack is disabled in the config", http.StatusServiceUnavailable)
		return
	}
	if config.Hijack.RequireAuth {
		needAuth := true
		user, passwd, ok := req.BasicAuth()
		if ok {
			for _, u := range config.Hijack.AuthUsers {
				if u.Username == user && comparePasswd(u.Password, passwd) {
					needAuth = false
					return
				}
			}
		}
		if needAuth {
			rw.Header().Set("WWW-Authenticate", `Basic realm="Login to access hijacked bmclapi", charset="UTF-8"`)
			http.Error(rw, "403 Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		if rec, err := h.fileMap.GetFileRecord(req.URL.Path); err == nil {
			ctx := req.Context()
			ctx = context.WithValue(ctx, "go-openbmclapi.handler.no.record.for.keepalive", true)
			h.downloadHandler(rw, req.WithContext(ctx), rec.Hash)
			return
		}
	}

	nowUnix := time.Now().Unix()

	cacheFileName := filepath.Join(config.Hijack.LocalCachePath, filepath.FromSlash(req.URL.Path))
	cached := h.getCache(req.URL.Path)
	if hjResponseWithCache(rw, req, cached, false) {
		return
	}

	u := *req.URL
	u.Scheme = "https"
	u.Host = hijackingHost
	req2, err := http.NewRequestWithContext(req.Context(), req.Method, u.String(), req.Body)
	if err != nil {
		http.Error(rw, "remote: "+err.Error(), http.StatusBadGateway)
		return
	}
	for k, v := range req.Header {
		req2.Header[k] = v
	}
	res, err := h.client.Do(req2)
	if err != nil {
		if hjResponseWithCache(rw, req, cached, true) {
			return
		}
		http.Error(rw, "remote: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	for k, v := range res.Header {
		rw.Header()[k] = v
	}
	if res.StatusCode/100 == 3 {
		// fix location header
		location, err := url.Parse(res.Header.Get("Location"))
		if err == nil && location.Host == "" && len(location.Path) >= 1 && location.Path[0] == '/' {
			location.Path = "/bmclapi" + location.Path
		}
		rw.Header().Set("Location", location.String())
	}
	rw.WriteHeader(res.StatusCode)
	var body io.Reader = res.Body
	if config.Hijack.EnableLocalCache && res.StatusCode == http.StatusOK {
		if exp, ok := utils.ParseCacheControl(res.Header.Get("Cache-Control")); ok {
			if exp > 0 {
				os.MkdirAll(filepath.Dir(cacheFileName), 0755)
				fd, err := os.OpenFile(cacheFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
				if err == nil {
					defer fd.Close()
					var n int64
					if n, err = io.Copy(fd, body); err == nil {
						if _, err = fd.Seek(0, io.SeekStart); err == nil {
							h.setCache(req.URL.Path, n, nowUnix+exp)
							body = fd
						}
					}
					if err != nil {
						fd.Close()
						os.Remove(cacheFileName)
					}
				}
			}
		}
	}
	io.Copy(rw, body)
}

type cacheStat struct {
	Size       int64 `json:"s"`
	ExpiresAt  int64 `json:"e"`
	ValidateAt int64 `json:"v"`
}

func (h *HjProxy) loadCache() (err error) {
	h.cache = make(map[string]*cacheStat)
	fd, err := os.Open(filepath.Join(config.Hijack.LocalCachePath, "__cache.json"))
	if err != nil {
		return
	}
	defer fd.Close()
	return json.NewDecoder(fd).Decode(&h.cache)
}

func (h *HjProxy) saveCache() (err error) {
	fd, err := os.Create(filepath.Join(config.Hijack.LocalCachePath, "__cache.json"))
	if err != nil {
		return
	}
	defer fd.Close()
	return json.NewEncoder(fd).Encode(h.cache)
}

func (h *HjProxy) getCache(path string) *cacheStat {
	h.cacheMux.RLock()
	defer h.cacheMux.RUnlock()
	stat, _ := h.cache[path]
	return stat
}

func (h *HjProxy) setCache(path string, size int64, exp int64) {
	h.cacheMux.Lock()
	defer h.cacheMux.Unlock()
	h.cache[path] = &cacheStat{
		Size:      size,
		ExpiresAt: exp,
	}
	if h.saveTimer == nil {
		h.saveTimer = time.AfterFunc(time.Second, func() {
			h.cacheMux.RLock()
			defer h.cacheMux.RUnlock()
			// it's safe to rlock here because the only other way to set it under a write lock
			h.saveTimer = nil
			h.saveCache()
		})
	} else {
		h.saveTimer.Reset(time.Second)
	}
}
