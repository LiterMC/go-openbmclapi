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
	"bytes"
	"context"
	"crypto"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/api/v0"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/utils"
)

func init() {
	// ignore TLS handshake error
	log.AddStdLogFilter(func(line []byte) bool {
		return bytes.HasPrefix(line, ([]byte)("http: TLS handshake error"))
	})
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
	return fmt.Sprintf("Serving %-15s | %-4s %s | %q", r.Addr, r.Method, r.URI, r.UA)
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
	var buf strings.Builder
	fmt.Fprintf(&buf, "Serve %3d | %12v | %7s | %-15s | %-4s %s | %q",
		r.Status, used, utils.BytesToUnit((float64)(r.Content)),
		r.Addr,
		r.Method, r.URI, r.UA)
	if len(r.Extra) > 0 {
		buf.WriteString(" | ")
		e := json.NewEncoder(&utils.NoLastNewLineWriter{Writer: &buf})
		e.SetEscapeHTML(false)
		e.Encode(r.Extra)
	}
	return buf.String()
}

var wsUpgrader = &websocket.Upgrader{
	HandshakeTimeout: time.Second * 30,
}

func (r *Runner) GetHandler() http.Handler {
	r.apiRateLimiter = limited.NewAPIRateMiddleWare(RealAddrCtxKey, loggedUserKey)
	r.apiRateLimiter.SetAnonymousRateLimit(r.RateLimit.Anonymous)
	r.apiRateLimiter.SetLoggedRateLimit(r.RateLimit.Logged)
	r.handlerAPIv0 = http.StripPrefix("/api/v0", v0.NewHandler(wsUpgrader))
	r.hijackHandler = http.StripPrefix("/bmclapi", r.hijackProxy)

	handler := utils.NewHttpMiddleWareHandler(r)
	// recover panic and log it
	handler.UseFunc(func(rw http.ResponseWriter, req *http.Request, next http.Handler) {
		defer log.RecoverPanic(func(any) {
			rw.WriteHeader(http.StatusInternalServerError)
		})
		next.ServeHTTP(rw, req)
	})
	handler.Use(r.apiRateLimiter)

	handler.Use(r.getRecordMiddleWare())
	return handler
}

func (r *Runner) getRecordMiddleWare() utils.MiddleWareFunc {
	type record struct {
		used   float64
		bytes  float64
		ua     string
		skipUA bool
	}
	recordCh := make(chan record, 1024)

	return func(rw http.ResponseWriter, req *http.Request, next http.Handler) {
		ua := req.UserAgent()
		var addr string
		if config.TrustedXForwardedFor {
			// X-Forwarded-For: <client>, <proxy1>, <proxy2>
			adr, _, _ := strings.Cut(req.Header.Get("X-Forwarded-For"), ",")
			addr = strings.TrimSpace(adr)
		}
		if addr == "" {
			addr, _, _ = net.SplitHostPort(req.RemoteAddr)
		}
		srw := utils.WrapAsStatusResponseWriter(rw)
		start := time.Now()

		log.LogAccess(log.LevelDebug, &preAccessRecord{
			Type:   "pre-access",
			Time:   start,
			Addr:   addr,
			Method: req.Method,
			URI:    req.RequestURI,
			UA:     ua,
		})

		extraInfoMap := make(map[string]any)
		ctx := req.Context()
		ctx = context.WithValue(ctx, RealAddrCtxKey, addr)
		ctx = context.WithValue(ctx, RealPathCtxKey, req.URL.Path)
		ctx = context.WithValue(ctx, AccessLogExtraCtxKey, extraInfoMap)
		req = req.WithContext(ctx)
		next.ServeHTTP(srw, req)

		used := time.Since(start)
		accRec := &accessRecord{
			Type:    "access",
			Status:  srw.Status,
			Used:    used,
			Content: srw.Wrote,
			Addr:    addr,
			Proto:   req.Proto,
			Method:  req.Method,
			URI:     req.RequestURI,
			UA:      ua,
		}
		if len(extraInfoMap) > 0 {
			accRec.Extra = extraInfoMap
		}
		log.LogAccess(log.LevelInfo, accRec)
	}
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

//go:embed robots.txt
var robotTxtContent string

func (r *Runner) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	method := req.Method
	u := req.URL

	rw.Header().Set("X-Powered-By", HeaderXPoweredBy)

	rawpath := u.EscapedPath()
	switch {
	case strings.HasPrefix(rawpath, "/download/"):
		if method != http.MethodGet && method != http.MethodHead {
			rw.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		hash := rawpath[len("/download/"):]
		if !utils.IsHex(hash) {
			http.Error(rw, hash+" is not a valid hash", http.StatusNotFound)
			return
		}

		for _, cr := range r.clusters {
			if cr.AcceptHost(req.Host) {
				cr.HandleFile(rw, req, hash)
				return
			}
		}
		http.Error(rw, "Host have not bind to a cluster", http.StatusNotFound)
		return
	case strings.HasPrefix(rawpath, "/measure/"):
		if method != http.MethodGet && method != http.MethodHead {
			rw.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		size, e := strconv.Atoi(rawpath[len("/measure/"):])
		if e != nil {
			http.Error(rw, e.Error(), http.StatusBadRequest)
			return
		} else if size < 0 || size > 200 {
			http.Error(rw, fmt.Sprintf("measure size %d out of range (0, 200]", size), http.StatusBadRequest)
			return
		}

		for _, cr := range r.clusters {
			if cr.AcceptHost(req.Host) {
				cr.HandleFile(rw, req, hash)
				return
			}
		}
		http.Error(rw, "Host have not bind to a cluster", http.StatusNotFound)
		return
	case rawpath == "/robots.txt":
		http.ServeContent(rw, req, "robots.txt", time.Time{}, strings.NewReader(robotTxtContent))
		return
	case strings.HasPrefix(rawpath, "/api/"):
		version, _, _ := strings.Cut(rawpath[len("/api/"):], "/")
		switch version {
		case "v0":
			r.handlerAPIv0.ServeHTTP(rw, req)
			return
		case "v1":
			r.handlerAPIv1.ServeHTTP(rw, req)
			return
		}
	case rawpath == "/" || rawpath == "/dashboard":
		http.Redirect(rw, req, "/dashboard/", http.StatusFound)
		return
	case strings.HasPrefix(rawpath, "/dashboard/"):
		if !r.DashboardEnabled {
			http.NotFound(rw, req)
			return
		}
		pth := rawpath[len("/dashboard/"):]
		r.serveDashboard(rw, req, pth)
		return
	case strings.HasPrefix(rawpath, "/bmclapi/"):
		r.hijackHandler.ServeHTTP(rw, req)
		return
	}
	http.NotFound(rw, req)
}
