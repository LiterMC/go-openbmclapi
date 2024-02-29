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

	"github.com/LiterMC/go-openbmclapi/database"
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
}

func NewHjProxy(client *http.Client, fileMap database.DB, downloadHandler downloadHandlerFn) (h *HjProxy) {
	return &HjProxy{
		client:          client,
		fileMap:         fileMap,
		downloadHandler: downloadHandler,
	}
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
		if rec, err := h.fileMap.Get(req.URL.Path); err == nil {
			ctx := req.Context()
			ctx = context.WithValue(ctx, "go-openbmclapi.handler.no.record.for.keepalive", true)
			h.downloadHandler(rw, req.WithContext(ctx), rec.Hash)
			return
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
	for k, v := range req.Header {
		req2.Header[k] = v
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
