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

package v0

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/utils"
)

func (h *Handler) buildDebugRoute(mux *http.ServeMux) {
	mux.HandleFunc("/log.io", h.routeLogIO)
	mux.Handle("/pprof", permHandleFunc(api.DebugPerm, h.routePprof))
	mux.Handle("GET /log_files", permHandleFunc(api.LogPerm, h.routeLogFiles))
	mux.Handle("GET /log_file/{file_name}", permHandleFunc(api.LogPerm, h.routeLogFile))
}

func (h *Handler) routeLogIO(rw http.ResponseWriter, req *http.Request) {
	addr := api.GetRequestRealAddr(req)

	conn, err := h.wsUpgrader.Upgrade(rw, req, nil)
	if err != nil {
		log.Debugf("[log.io]: Websocket upgrade error: %v", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	cli := apiGetClientId(req)

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	conn.SetReadLimit(1024 * 4)
	pongTimeoutTimer := time.NewTimer(time.Second * 75)
	go func() {
		defer conn.Close()
		defer cancel()
		defer pongTimeoutTimer.Stop()
		select {
		case _, ok := <-pongTimeoutTimer.C:
			if !ok {
				return
			}
			log.Error("[log.io]: Did not receive packet from client longer than 75s")
			return
		case <-ctx.Done():
			return
		}
	}()

	var authData struct {
		Token string `json:"token"`
	}
	deadline := time.Now().Add(time.Second * 10)
	conn.SetReadDeadline(deadline)
	err = conn.ReadJSON(&authData)
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		if time.Now().After(deadline) {
			conn.WriteJSON(Map{
				"type":    "error",
				"message": "auth timeout",
			})
		} else {
			conn.WriteJSON(Map{
				"type":    "error",
				"message": "unexpected auth data: " + err.Error(),
			})
		}
		return
	}
	if _, _, err = h.tokens.VerifyAuthToken(cli, authData.Token); err != nil {
		conn.WriteJSON(Map{
			"type":    "error",
			"message": "auth failed",
		})
		return
	}
	if err := conn.WriteJSON(Map{
		"type": "ready",
	}); err != nil {
		return
	}

	var level atomic.Int32
	level.Store((int32)(log.LevelInfo))

	type logObj struct {
		Type  string `json:"type"`
		Time  int64  `json:"time"` // UnixMilli
		Level string `json:"lvl"`
		Log   string `json:"log"`
	}
	c := make(chan *logObj, 64)
	unregister := log.RegisterLogMonitor(log.LevelDebug, func(ts int64, l log.Level, msg string) {
		if (log.Level)(level.Load()) > l&log.LevelMask {
			return
		}
		select {
		case c <- &logObj{
			Type:  "log",
			Time:  ts,
			Level: l.String(),
			Log:   msg,
		}:
		default:
		}
	})
	defer unregister()

	go func() {
		defer log.RecoverPanic(nil)
		defer conn.Close()
		defer cancel()
		var data map[string]any
		for {
			clear(data)
			if err := conn.ReadJSON(&data); err != nil {
				log.Errorf("[log.io]: Cannot read from peer: %v", err)
				return
			}
			typ, ok := data["type"].(string)
			if !ok {
				continue
			}
			switch typ {
			case "pong":
				log.Debugf("[log.io]: received PONG from %s: %v", addr, data["data"])
				pongTimeoutTimer.Reset(time.Second * 75)
			case "set-level":
				l, ok := data["level"].(string)
				if ok {
					switch l {
					case "DBUG":
						level.Store((int32)(log.LevelDebug))
					case "INFO":
						level.Store((int32)(log.LevelInfo))
					case "WARN":
						level.Store((int32)(log.LevelWarn))
					case "ERRO":
						level.Store((int32)(log.LevelError))
					default:
						continue
					}
					select {
					case c <- &logObj{
						Type:  "log",
						Time:  time.Now().UnixMilli(),
						Level: log.LevelInfo.String(),
						Log:   "[dashboard]: Set log level to " + l + " for this log.io",
					}:
					default:
					}
				}
			}
		}
	}()

	sendMsgCh := make(chan any, 64)
	go func() {
		for {
			select {
			case v := <-c:
				select {
				case sendMsgCh <- v:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	pingTicker := time.NewTicker(time.Second * 45)
	defer pingTicker.Stop()
	forceSendTimer := time.NewTimer(time.Second)
	if !forceSendTimer.Stop() {
		<-forceSendTimer.C
	}

	batchMsg := make([]any, 0, 64)
	for {
		select {
		case v := <-sendMsgCh:
			batchMsg = append(batchMsg, v)
			forceSendTimer.Reset(time.Second)
		WAIT_MORE:
			for {
				select {
				case v := <-sendMsgCh:
					batchMsg = append(batchMsg, v)
				case <-time.After(time.Millisecond * 20):
					if !forceSendTimer.Stop() {
						<-forceSendTimer.C
					}
					break WAIT_MORE
				case <-forceSendTimer.C:
					break WAIT_MORE
				case <-ctx.Done():
					forceSendTimer.Stop()
					return
				}
			}
			if len(batchMsg) == 1 {
				if err := conn.WriteJSON(batchMsg[0]); err != nil {
					return
				}
			} else {
				if err := conn.WriteJSON(batchMsg); err != nil {
					return
				}
			}
			// release objects
			for i, _ := range batchMsg {
				batchMsg[i] = nil
			}
			batchMsg = batchMsg[:0]
		case <-pingTicker.C:
			if err := conn.WriteJSON(Map{
				"type": "ping",
				"data": time.Now().UnixMilli(),
			}); err != nil {
				log.Errorf("[log.io]: Error when sending ping packet: %v", err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler) routePprof(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		errorMethodNotAllowed(rw, req, http.MethodGet)
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
	if debug == 1 {
		fmt.Fprintf(rw, "version: %s (%s)\n", build.BuildVersion, build.ClusterVersion)
	}
	p.WriteTo(rw, debug)
}

func (h *Handler) routeLogFiles(rw http.ResponseWriter, req *http.Request) {
	files := log.ListLogs()
	type FileInfo struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	data := make([]FileInfo, 0, len(files))
	for _, file := range files {
		if s, err := os.Stat(filepath.Join(log.BaseDir(), file)); err == nil {
			data = append(data, FileInfo{
				Name: file,
				Size: s.Size(),
			})
		}
	}
	writeJson(rw, http.StatusOK, Map{
		"files": data,
	})
}

func (h *Handler) routeLogFile(rw http.ResponseWriter, req *http.Request) {
	fileName := req.PathValue("file_name")
	query := req.URL.Query()
	fd, err := os.Open(filepath.Join(log.BaseDir(), fileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJson(rw, http.StatusNotFound, Map{
				"error":   "file not exists",
				"message": "Cannot find log file",
				"path":    req.URL.Path,
			})
			return
		}
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "cannot open file",
			"message": err.Error(),
		})
		return
	}
	defer fd.Close()
	name := filepath.Base(req.URL.Path)
	isGzip := filepath.Ext(name) == ".gz"
	if query.Get("no_encrypt") == "1" {
		var modTime time.Time
		if stat, err := fd.Stat(); err == nil {
			modTime = stat.ModTime()
		}
		rw.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
		if isGzip {
			rw.Header().Set("Content-Type", "application/octet-stream")
		} else {
			rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		http.ServeContent(rw, req, name, modTime, fd)
	} else {
		if !isGzip {
			name += ".gz"
		}
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+".encrypted"))
		h.routeLogFileEncrypted(rw, req, fd, !isGzip)
	}
}

func (h *Handler) routeLogFileEncrypted(rw http.ResponseWriter, req *http.Request, r io.Reader, useGzip bool) {
	rw.WriteHeader(http.StatusOK)
	if req.Method == http.MethodHead {
		return
	}
	if useGzip {
		pr, pw := io.Pipe()
		defer pr.Close()
		go func(r io.Reader) {
			gw := gzip.NewWriter(pw)
			if _, err := io.Copy(gw, r); err != nil {
				pw.CloseWithError(err)
				return
			}
			if err := gw.Close(); err != nil {
				pw.CloseWithError(err)
				return
			}
			pw.Close()
		}(r)
		r = pr
	}
	if err := utils.EncryptStream(rw, r, utils.DeveloporPublicKey); err != nil {
		log.Errorf("Cannot write encrypted log stream: %v", err)
	}
}
