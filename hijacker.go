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
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
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

type HjProxy struct {
	dialer *net.Dialer
	path   string
	client *http.Client
}

func NewHjProxy(dialer *net.Dialer, path string) (h *HjProxy) {
	return &HjProxy{
		dialer: dialer,
		path:   path,
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: dialer.DialContext,
			},
		},
	}
}

const hijackingHost = "bmclapi2.bangbang93.com"

func (h *HjProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		target := filepath.Join(h.path, filepath.Clean(filepath.FromSlash(req.URL.Path)))
		fd, err := os.Open(target)
		if err == nil {
			defer fd.Close()
			if stat, err := fd.Stat(); err == nil {
				if !stat.IsDir() {
					modTime := stat.ModTime()
					http.ServeContent(rw, req, filepath.Base(target), modTime, fd)
					return
				}
			}
		}
	}

	u := *req.URL
	u.Scheme = "https"
	u.Host = hijackingHost
	req2, err := http.NewRequestWithContext(req.Context(), req.Method, u.String(), req.Body)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	res, err := h.client.Do(req2)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	for k, v := range res.Header {
		rw.Header()[k] = v
	}
	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
}
