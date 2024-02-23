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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"runtime/pprof"
)

func (cr *Cluster) verifyDashBoardToken(token string) bool {
	return false
}

func (cr *Cluster) apiAuthHandle(next http.Handler) http.Handler {
	return (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get("Authorization")
		tk, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok || cr.verifyDashBoardToken(tk) {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "invalid authorization token",
			})
			return
		}
		next.ServeHTTP(rw, req)
	})
}

func (cr *Cluster) apiAuthHandleFunc(next http.HandlerFunc) http.Handler {
	return cr.apiAuthHandle(next)
}

func (cr *Cluster) initAPIv0() (mux *http.ServeMux) {
	mux = http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "404 not found",
			"path":  req.URL.Path,
		})
	})
	mux.HandleFunc("/ping", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusOK, Map{
			"version": BuildVersion,
			"time":    time.Now(),
		})
	})
	mux.HandleFunc("/status", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusOK, Map{
			"startAt": startTime,
			"stats":   &cr.stats,
			"enabled": cr.enabled.Load(),
		})
	})
	mux.Handle("/log", cr.apiAuthHandleFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		e := json.NewEncoder(rw)
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()

		level := LogLevelInfo
		if strings.ToLower(req.URL.Query().Get("level")) == "debug" {
			level = LogLevelDebug
		}

		unregister := RegisterLogMonitor(level, func(ts int64, level LogLevel, log string) {
			type logObj struct {
				Time  int64  `json:"time"`
				Level string `json:"lvl"`
				Log   string `json:"log"`
			}
			var v = logObj{
				Time:  ts,
				Level: level.String(),
				Log:   log,
			}
			if err := e.Encode(v); err != nil {
				e.Encode(err.Error())
				cancel()
				return
			}
		})
		defer unregister()

		select {
		case <-ctx.Done():
		}
	}))
	mux.Handle("/pprof", cr.apiAuthHandleFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			rw.Header().Set("Allow", http.MethodGet)
			http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		query := req.URL.Query()
		lookup := query.Get("lookup")
		p := pprof.Lookup(lookup)
		if p == nil {
			http.Error(rw, fmt.Sprintf("pprof.Lookup(%q) returned nil", lookup), http.StatusBadRequest)
			return
		}
		view := query.Get("view")
		debug, err := strconv.Atoi(query.Get("debug"))
		if err != nil {
			debug = 1
		}
		if debug == 1 {
			rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		} else {
			rw.Header().Set("Content-Type", "application/octet-stream")
		}
		if view != "1" {
			name := fmt.Sprintf(time.Now().Format("dump-%s-20060102-150405"), lookup)
			if debug == 1 {
				name += ".txt"
			} else {
				name += ".dump"
			}
			rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		}
		rw.WriteHeader(http.StatusOK)
		p.WriteTo(rw, debug)
	}))
	return
}

type Map = map[string]any

func writeJson(rw http.ResponseWriter, code int, data any) (err error) {
	buf, err := json.Marshal(data)
	if err != nil {
		http.Error(rw, "Error when encoding response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Content-Length", strconv.Itoa(len(buf)))
	rw.WriteHeader(code)
	_, err = rw.Write(buf)
	return
}
