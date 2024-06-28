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
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/schema"
	"runtime/pprof"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/notify"
	"github.com/LiterMC/go-openbmclapi/utils"
)

const (
	clientIdCookieName = "_id"

	clientIdKey = "go-openbmclapi.cluster.client.id"
)

func apiGetClientId(req *http.Request) (id string) {
	return req.Context().Value(clientIdKey).(string)
}

type Handler struct {
	handler       *utils.HttpMiddleWareHandler
	router        *http.ServeMux
	users         api.UserManager
	tokens        api.TokenManager
	subscriptions api.SubscriptionManager
}

var _ http.Handler = (*Handler)(nil)

func NewHandler(
	users api.UserManager,
	tokenManager api.TokenManager,
	subManager api.SubscriptionManager,
) *Handler {
	mux := http.NewServeMux()
	h := &Handler{
		router:        mux,
		handler:       utils.NewHttpMiddleWareHandler(mux),
		users:         users,
		tokens:        tokenManager,
		subscriptions: subManager,
	}
	h.buildRoute()
	h.handler.Use(cliIdMiddleWare)
	h.handler.Use(h.authMiddleWare)
	return h
}

func (h *Handler) Handler() *utils.HttpMiddleWareHandler {
	return h.handler
}

func (h *Handler) buildRoute() {
	mux := h.router

	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "404 not found",
			"path":  req.URL.Path,
		})
	})

	mux.HandleFunc("/ping", h.routePing)
	mux.HandleFunc("/status", h.routeStatus)
	mux.Handle("/stat/", http.StripPrefix("/stat/", (http.HandlerFunc)(h.routeStat)))

	mux.HandleFunc("/challenge", h.routeChallenge)
	mux.HandleFunc("/login", h.routeLogin)
	mux.Handle("/requestToken", authHandleFunc(h.routeRequestToken))
	mux.Handle("/logout", authHandleFunc(h.routeLogout))

	mux.HandleFunc("/log.io", h.routeLogIO)
	mux.Handle("/pprof", permHandleFunc(api.DebugPerm, h.routePprof))
	mux.HandleFunc("/subscribeKey", h.routeSubscribeKey)
	mux.Handle("/subscribe", permHandleFunc(api.SubscribePerm, &utils.HttpMethodHandler{
		Get:    h.routeSubscribeGET,
		Post:   h.routeSubscribePOST,
		Delete: h.routeSubscribeDELETE,
	}))
	mux.Handle("/subscribe_email", permHandleFunc(api.SubscribePerm, &utils.HttpMethodHandler{
		Get:    h.routeSubscribeEmailGET,
		Post:   h.routeSubscribeEmailPOST,
		Patch:  h.routeSubscribeEmailPATCH,
		Delete: h.routeSubscribeEmailDELETE,
	}))
	mux.Handle("/webhook", permHandleFunc(api.SubscribePerm, &utils.HttpMethodHandler{
		Get:    h.routeWebhookGET,
		Post:   h.routeWebhookPOST,
		Patch:  h.routeWebhookPATCH,
		Delete: h.routeWebhookDELETE,
	}))

	mux.Handle("/log_files", permHandleFunc(api.LogPerm, h.routeLogFiles))
	mux.Handle("/log_file/", permHandle(api.LogPerm, http.StripPrefix("/log_file/", (http.HandlerFunc)(h.routeLogFile))))

	mux.Handle("/configure/cluster", permHandleFunc(api.ClusterPerm, h.routeConfigureCluster))
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.handler.ServeHTTP(rw, req)
}

func (h *Handler) routePing(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		errorMethodNotAllowed(rw, req, http.MethodGet)
		return
	}
	limited.SetSkipRateLimit(req)
	authed := getRequestTokenType(req) == tokenTypeAuth
	writeJson(rw, http.StatusOK, Map{
		"version": build.BuildVersion,
		"time":    time.Now().UnixMilli(),
		"authed":  authed,
	})
}

func (cr *Cluster) routeStatus(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		errorMethodNotAllowed(rw, req, http.MethodGet)
		return
	}
	limited.SetSkipRateLimit(req)
	type syncData struct {
		Prog  int64 `json:"prog"`
		Total int64 `json:"total"`
	}
	type statusData struct {
		StartAt  time.Time     `json:"startAt"`
		Stats    *notify.Stats `json:"stats"`
		Enabled  bool          `json:"enabled"`
		IsSync   bool          `json:"isSync"`
		Sync     *syncData     `json:"sync,omitempty"`
		Storages []string      `json:"storages"`
	}
	storages := make([]string, len(cr.storageOpts))
	for i, opt := range cr.storageOpts {
		storages[i] = opt.Id
	}
	status := statusData{
		StartAt:  startTime,
		Stats:    &cr.stats,
		Enabled:  cr.enabled.Load(),
		IsSync:   cr.issync.Load(),
		Storages: storages,
	}
	if status.IsSync {
		status.Sync = &syncData{
			Prog:  cr.syncProg.Load(),
			Total: cr.syncTotal.Load(),
		}
	}
	writeJson(rw, http.StatusOK, &status)
}

func (cr *Cluster) routeStat(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		errorMethodNotAllowed(rw, req, http.MethodGet)
		return
	}
	limited.SetSkipRateLimit(req)
	name := req.URL.Path
	if name == "" {
		rw.Header().Set("Cache-Control", "public, max-age=60")
		writeJson(rw, http.StatusOK, &cr.stats)
		return
	}
	data, err := cr.stats.MarshalSubStat(name)
	if err != nil {
		http.Error(rw, "Error when encoding response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Cache-Control", "public, max-age=30")
	writeJson(rw, http.StatusOK, (json.RawMessage)(data))
}

func (h *Handler) routeChallenge(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		errorMethodNotAllowed(rw, req, http.MethodGet)
		return
	}
	cli := apiGetClientId(req)
	query := req.URL.Query()
	action := query.Get("action")
	token, err := h.generateChallengeToken(cli, action)
	if err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "Cannot generate token",
			"message": err.Error(),
		})
		return
	}
	writeJson(rw, http.StatusOK, Map{
		"token": token,
	})
}

func (h *Handler) routeLogin(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		errorMethodNotAllowed(rw, req, http.MethodPost)
		return
	}
	if !config.Dashboard.Enable {
		writeJson(rw, http.StatusServiceUnavailable, Map{
			"error": "dashboard is disabled in the config",
		})
		return
	}
	cli := apiGetClientId(req)

	var data struct {
		User      string `json:"username" schema:"username"`
		Challenge string `json:"challenge" schema:"challenge"`
		Signature string `json:"signature" schema:"signature"`
	}
	if !parseRequestBody(rw, req, &data) {
		return
	}

	if err := h.tokens.VerifyChallengeToken(cli, "login", data.Challenge); err != nil {
		writeJson(rw, http.StatusUnauthorized, Map{
			"error": "Invalid challenge",
		})
		return
	}
	if err := h.tokens.VerifyUserPassword(data.User, func(password string) bool {
		expectSignature := utils.HMACSha256HexBytes(password, data.Challenge)
		return subtle.ConstantTimeCompare(expectSignature, ([]byte)(data.Signature)) == 0
	}); err != nil {
		writeJson(rw, http.StatusUnauthorized, Map{
			"error": "The username or password is incorrect",
		})
		return
	}
	token, err := cr.generateAuthToken(cli, data.User)
	if err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "Cannot generate token",
			"message": err.Error(),
		})
		return
	}
	writeJson(rw, http.StatusOK, Map{
		"token": token,
	})
}

func (cr *Cluster) routeRequestToken(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		errorMethodNotAllowed(rw, req, http.MethodPost)
		return
	}
	defer req.Body.Close()
	if getRequestTokenType(req) != tokenTypeAuth {
		writeJson(rw, http.StatusUnauthorized, Map{
			"error": "invalid authorization type",
		})
		return
	}

	var payload struct {
		Path  string            `json:"path"`
		Query map[string]string `json:"query,omitempty"`
	}
	if !parseRequestBody(rw, req, &payload) {
		return
	}
	log.Debugf("payload: %#v", payload)
	if payload.Path == "" || payload.Path[0] != '/' {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "path is invalid",
			"message": "'path' must be a non empty string which starts with '/'",
		})
		return
	}
	cli := apiGetClientId(req)
	user := getLoggedUser(req)
	token, err := cr.generateAPIToken(cli, user, payload.Path, payload.Query)
	if err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "cannot generate token",
			"message": err.Error(),
		})
		return
	}
	writeJson(rw, http.StatusOK, Map{
		"token": token,
	})
}

func (cr *Cluster) routeLogout(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		errorMethodNotAllowed(rw, req, http.MethodPost)
		return
	}
	limited.SetSkipRateLimit(req)
	tid := req.Context().Value(tokenIdKey).(string)
	cr.database.RemoveJTI(tid)
	rw.WriteHeader(http.StatusNoContent)
}

func (cr *Cluster) routeLogIO(rw http.ResponseWriter, req *http.Request) {
	addr, _ := req.Context().Value(RealAddrCtxKey).(string)

	conn, err := cr.wsUpgrader.Upgrade(rw, req, nil)
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
	if _, _, err = cr.verifyAuthToken(cli, authData.Token); err != nil {
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

func (cr *Cluster) routePprof(rw http.ResponseWriter, req *http.Request) {
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

func (cr *Cluster) routeWebhookGET(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	if sid := req.URL.Query().Get("id"); sid != "" {
		id, err := uuid.Parse(sid)
		if err != nil {
			writeJson(rw, http.StatusBadRequest, Map{
				"error":   "uuid format error",
				"message": err.Error(),
			})
			return
		}
		record, err := cr.database.GetWebhook(user, id)
		if err != nil {
			if err == database.ErrNotFound {
				writeJson(rw, http.StatusNotFound, Map{
					"error": "no webhook was found",
				})
				return
			}
			writeJson(rw, http.StatusInternalServerError, Map{
				"error":   "database error",
				"message": err.Error(),
			})
			return
		}
		writeJson(rw, http.StatusOK, record)
		return
	}
	records := make([]database.WebhookRecord, 0, 4)
	if err := cr.database.ForEachUsersWebhook(user, func(rec *database.WebhookRecord) error {
		records = append(records, *rec)
		return nil
	}); err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "database error",
			"message": err.Error(),
		})
		return
	}
	writeJson(rw, http.StatusOK, records)
}

func (cr *Cluster) routeWebhookPOST(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	var data database.WebhookRecord
	if !parseRequestBody(rw, req, &data) {
		return
	}

	data.User = user
	if err := cr.database.AddWebhook(data); err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "Database update failed",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusCreated)
}

func (cr *Cluster) routeWebhookPATCH(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	id := req.URL.Query().Get("id")
	var data database.WebhookRecord
	if !parseRequestBody(rw, req, &data) {
		return
	}
	data.User = user
	var err error
	if data.Id, err = uuid.Parse(id); err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "uuid format error",
			"message": err.Error(),
		})
		return
	}
	if err := cr.database.UpdateWebhook(data); err != nil {
		if err == database.ErrNotFound {
			writeJson(rw, http.StatusNotFound, Map{
				"error": "no webhook was found",
			})
			return
		}
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "database error",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (cr *Cluster) routeWebhookDELETE(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	id, err := uuid.Parse(req.URL.Query().Get("id"))
	if err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "uuid format error",
			"message": err.Error(),
		})
		return
	}
	if err := cr.database.RemoveWebhook(user, id); err != nil {
		if err == database.ErrNotFound {
			writeJson(rw, http.StatusNotFound, Map{
				"error": "no webhook was found",
			})
			return
		}
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "database error",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (cr *Cluster) routeLogFiles(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		errorMethodNotAllowed(rw, req, http.MethodGet)
		return
	}
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

func (cr *Cluster) routeLogFile(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		errorMethodNotAllowed(rw, req, http.MethodGet+", "+http.MethodHead)
		return
	}
	query := req.URL.Query()
	fd, err := os.Open(filepath.Join(log.BaseDir(), req.URL.Path))
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
		cr.routeLogFileEncrypted(rw, req, fd, !isGzip)
	}
}

func (cr *Cluster) routeLogFileEncrypted(rw http.ResponseWriter, req *http.Request, r io.Reader, useGzip bool) {
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

type Map = map[string]any

var errUnknownContent = errors.New("unknown content-type")
var formDecoder = schema.NewDecoder()

func parseRequestBody(rw http.ResponseWriter, req *http.Request, ptr any) (parsed bool) {
	contentType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":        "Unexpected Content-Type",
			"content-type": req.Header.Get("Content-Type"),
			"message":      err.Error(),
		})
		return
	}
	switch contentType {
	case "application/json":
		if err := json.NewDecoder(req.Body).Decode(ptr); err != nil {
			writeJson(rw, http.StatusBadRequest, Map{
				"error":   "Cannot decode request body",
				"message": err.Error(),
			})
			return
		}
		return true
	case "application/x-www-form-urlencoded":
		if err := req.ParseForm(); err != nil {
			writeJson(rw, http.StatusBadRequest, Map{
				"error":   "Cannot decode request body",
				"message": err.Error(),
			})
			return
		}
		if err := formDecoder.Decode(ptr, req.PostForm); err != nil {
			writeJson(rw, http.StatusBadRequest, Map{
				"error":   "Cannot decode request body",
				"message": err.Error(),
			})
			return
		}
		return true
	default:
		writeJson(rw, http.StatusBadRequest, Map{
			"error":        "Unexpected Content-Type",
			"content-type": contentType,
		})
		return
	}
}

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

func errorMethodNotAllowed(rw http.ResponseWriter, req *http.Request, allow string) {
	rw.Header().Set("Allow", allow)
	rw.WriteHeader(http.StatusMethodNotAllowed)
	return true
}
