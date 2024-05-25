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
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"runtime/pprof"
	// "github.com/gorilla/websocket"
	"github.com/google/uuid"

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

func (cr *Cluster) cliIdHandle(next http.Handler) http.Handler {
	return (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
		var id string
		if cid, _ := req.Cookie(clientIdCookieName); cid != nil {
			id = cid.Value
		} else {
			var err error
			id, err = utils.GenRandB64(16)
			if err != nil {
				http.Error(rw, "cannot generate random number", http.StatusInternalServerError)
				return
			}
			http.SetCookie(rw, &http.Cookie{
				Name:     clientIdCookieName,
				Value:    id,
				Expires:  time.Now().Add(time.Hour * 24 * 365 * 16),
				Secure:   true,
				HttpOnly: true,
			})
		}
		req = req.WithContext(context.WithValue(req.Context(), clientIdKey, utils.AsSha256(id)))
		next.ServeHTTP(rw, req)
	})
}

func (cr *Cluster) authMiddleware(rw http.ResponseWriter, req *http.Request, next http.Handler) {
	cli := apiGetClientId(req)

	ctx := req.Context()

	var (
		id  string
		uid string
		err error
	)
	if req.Method == http.MethodGet {
		if tk := req.URL.Query().Get("_t"); tk != "" {
			path := GetRequestRealPath(req)
			if id, uid, err = cr.verifyAPIToken(cli, tk, path, req.URL.Query()); err == nil {
				ctx = context.WithValue(ctx, tokenTypeKey, tokenTypeAPI)
			}
		}
	}
	if id == "" {
		auth := req.Header.Get("Authorization")
		tk, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok {
			if err == nil {
				err = ErrUnsupportAuthType
			}
		} else if id, uid, err = cr.verifyAuthToken(cli, tk); err != nil {
			id = ""
		} else {
			ctx = context.WithValue(ctx, tokenTypeKey, tokenTypeAuth)
		}
	}
	if id != "" {
		ctx = context.WithValue(ctx, loggedUserKey, uid)
		ctx = context.WithValue(ctx, tokenIdKey, id)
		req = req.WithContext(ctx)
	}
	next.ServeHTTP(rw, req)
}

func (cr *Cluster) apiAuthHandle(next http.Handler) http.Handler {
	return (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
		if req.Context().Value(tokenTypeKey) == nil {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "403 Unauthorized",
			})
			return
		}
		next.ServeHTTP(rw, req)
	})
}

func (cr *Cluster) apiAuthHandleFunc(next http.HandlerFunc) http.Handler {
	return cr.apiAuthHandle(next)
}

func (cr *Cluster) initAPIv0() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "404 not found",
			"path":  req.URL.Path,
		})
	})

	mux.HandleFunc("/ping", cr.apiV0Ping)
	mux.HandleFunc("/status", cr.apiV0Status)
	mux.HandleFunc("/stat", cr.apiV0Stat)

	mux.HandleFunc("/login", cr.apiV0Login)
	mux.Handle("/requestToken", cr.apiAuthHandleFunc(cr.apiV0RequestToken))
	mux.Handle("/logout", cr.apiAuthHandleFunc(cr.apiV0Logout))

	mux.HandleFunc("/log.io", cr.apiV0LogIO)
	mux.Handle("/pprof", cr.apiAuthHandleFunc(cr.apiV0Pprof))
	mux.HandleFunc("/subscribeKey", cr.apiV0SubscribeKey)
	mux.Handle("/subscribe", cr.apiAuthHandleFunc(cr.apiV0Subscribe))
	mux.Handle("/subscribe_email", cr.apiAuthHandleFunc(cr.apiV0SubscribeEmail))
	mux.Handle("/webhook", cr.apiAuthHandleFunc(cr.apiV0Webhook))

	next := cr.apiRateLimiter.WrapHandler(mux)
	return (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
		cr.authMiddleware(rw, req, next)
	})
}

func (cr *Cluster) apiV0Ping(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodGet) {
		return
	}
	limited.SetSkipRateLimit(req)
	authed := getRequestTokenType(req) == tokenTypeAuth
	writeJson(rw, http.StatusOK, Map{
		"version": build.BuildVersion,
		"time":    time.Now(),
		"authed":  authed,
	})
}

func (cr *Cluster) apiV0Status(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodGet) {
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

func (cr *Cluster) apiV0Stat(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodGet) {
		return
	}
	limited.SetSkipRateLimit(req)
	query := req.URL.Query()
	name := query.Get("name")
	if name == "" {
		writeJson(rw, http.StatusOK, &cr.stats)
		return
	}
	data, err := cr.stats.MarshalSubStat(name)
	if err != nil {
		http.Error(rw, "Error when encoding response: "+err.Error(), http.StatusInternalServerError)
	}
	writeJson(rw, http.StatusOK, (json.RawMessage)(data))
}

func (cr *Cluster) apiV0Login(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodPost) {
		return
	}
	if !config.Dashboard.Enable {
		writeJson(rw, http.StatusServiceUnavailable, Map{
			"error": "dashboard is disabled in the config",
		})
		return
	}
	cli := apiGetClientId(req)

	type T = struct {
		User string `json:"username"`
		Pass string `json:"password"`
	}
	data, ok := parseRequestBody(rw, req, func(rw http.ResponseWriter, req *http.Request, ct string, data *T) error {
		switch ct {
		case "application/x-www-form-urlencoded":
			data.User = req.PostFormValue("username")
			data.Pass = req.PostFormValue("password")
			return nil
		default:
			return errUnknownContent
		}
	})
	if !ok {
		return
	}

	expectUsername, expectPassword := config.Dashboard.Username, config.Dashboard.Password
	if expectUsername == "" || expectPassword == "" {
		writeJson(rw, http.StatusUnauthorized, Map{
			"error": "The username or password was not set on the server",
		})
		return
	}
	expectPassword = utils.AsSha256Hex(expectPassword)
	if subtle.ConstantTimeCompare(([]byte)(expectUsername), ([]byte)(data.User)) == 0 ||
		subtle.ConstantTimeCompare(([]byte)(expectPassword), ([]byte)(data.Pass)) == 0 {
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

func (cr *Cluster) apiV0RequestToken(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodPost) {
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
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "cannot decode payload in json format",
			"message": err.Error(),
		})
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

func (cr *Cluster) apiV0Logout(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodPost) {
		return
	}
	limited.SetSkipRateLimit(req)
	tid := req.Context().Value(tokenIdKey).(string)
	cr.database.RemoveJTI(tid)
	rw.WriteHeader(http.StatusNoContent)
}

func (cr *Cluster) apiV0LogIO(rw http.ResponseWriter, req *http.Request) {
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

func (cr *Cluster) apiV0Pprof(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodGet) {
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

func (cr *Cluster) apiV0SubscribeKey(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodGet) {
		return
	}
	key := cr.webpushKeyB64
	etag := `"` + utils.AsSha256(key) + `"`
	rw.Header().Set("ETag", etag)
	if cachedTag := req.Header.Get("If-None-Match"); cachedTag == etag {
		rw.WriteHeader(http.StatusNotModified)
		return
	}
	writeJson(rw, http.StatusOK, Map{
		"publicKey": key,
	})
}

func (cr *Cluster) apiV0Subscribe(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodGet, http.MethodPost, http.MethodDelete) {
		return
	}
	cliId := apiGetClientId(req)
	user := getLoggedUser(req)
	if user == "" {
		writeJson(rw, http.StatusForbidden, Map{
			"error": "Unauthorized",
		})
		return
	}
	switch req.Method {
	case http.MethodGet:
		cr.apiV0SubscribeGET(rw, req, user, cliId)
	case http.MethodPost:
		cr.apiV0SubscribePOST(rw, req, user, cliId)
	case http.MethodDelete:
		cr.apiV0SubscribeDELETE(rw, req, user, cliId)
	default:
		panic("unreachable")
	}
}

func (cr *Cluster) apiV0SubscribeGET(rw http.ResponseWriter, req *http.Request, user string, client string) {
	record, err := cr.database.GetSubscribe(user, client)
	if err != nil {
		if err == database.ErrNotFound {
			writeJson(rw, http.StatusNotFound, Map{
				"error": "no subscription was found",
			})
			return
		}
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "database error",
			"message": err.Error(),
		})
		return
	}
	writeJson(rw, http.StatusOK, Map{
		"scopes":   record.Scopes,
		"reportAt": record.ReportAt,
	})
}

func (cr *Cluster) apiV0SubscribePOST(rw http.ResponseWriter, req *http.Request, user string, client string) {
	data, ok := parseRequestBody[database.SubscribeRecord](rw, req, nil)
	if !ok {
		return
	}
	data.User = user
	data.Client = client
	if err := cr.database.SetSubscribe(data); err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "Database update failed",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (cr *Cluster) apiV0SubscribeDELETE(rw http.ResponseWriter, req *http.Request, user string, client string) {
	if err := cr.database.RemoveSubscribe(user, client); err != nil {
		if err == database.ErrNotFound {
			writeJson(rw, http.StatusNotFound, Map{
				"error": "no subscription was found",
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

func (cr *Cluster) apiV0SubscribeEmail(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete) {
		return
	}
	user := getLoggedUser(req)
	if user == "" {
		writeJson(rw, http.StatusForbidden, Map{
			"error": "Unauthorized",
		})
		return
	}
	switch req.Method {
	case http.MethodGet:
		cr.apiV0SubscribeEmailGET(rw, req, user)
	case http.MethodPost:
		cr.apiV0SubscribeEmailPOST(rw, req, user)
	case http.MethodPatch:
		cr.apiV0SubscribeEmailPATCH(rw, req, user)
	case http.MethodDelete:
		cr.apiV0SubscribeEmailDELETE(rw, req, user)
	default:
		panic("unreachable")
	}
}

func (cr *Cluster) apiV0SubscribeEmailGET(rw http.ResponseWriter, req *http.Request, user string) {
	if addr := req.URL.Query().Get("addr"); addr != "" {
		record, err := cr.database.GetEmailSubscription(user, addr)
		if err != nil {
			if err == database.ErrNotFound {
				writeJson(rw, http.StatusNotFound, Map{
					"error": "no email subscription was found",
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
	records := make([]database.EmailSubscriptionRecord, 0, 4)
	if err := cr.database.ForEachUsersEmailSubscription(user, func(rec *database.EmailSubscriptionRecord) error {
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

func (cr *Cluster) apiV0SubscribeEmailPOST(rw http.ResponseWriter, req *http.Request, user string) {
	data, ok := parseRequestBody[database.EmailSubscriptionRecord](rw, req, nil)
	if !ok {
		return
	}

	data.User = user
	if err := cr.database.AddEmailSubscription(data); err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "Database update failed",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusCreated)
}

func (cr *Cluster) apiV0SubscribeEmailPATCH(rw http.ResponseWriter, req *http.Request, user string) {
	addr := req.URL.Query().Get("addr")
	data, ok := parseRequestBody[database.EmailSubscriptionRecord](rw, req, nil)
	if !ok {
		return
	}
	data.User = user
	data.Addr = addr
	if err := cr.database.UpdateEmailSubscription(data); err != nil {
		if err == database.ErrNotFound {
			writeJson(rw, http.StatusNotFound, Map{
				"error": "no email subscription was found",
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

func (cr *Cluster) apiV0SubscribeEmailDELETE(rw http.ResponseWriter, req *http.Request, user string) {
	addr := req.URL.Query().Get("addr")
	if err := cr.database.RemoveEmailSubscription(user, addr); err != nil {
		if err == database.ErrNotFound {
			writeJson(rw, http.StatusNotFound, Map{
				"error": "no email subscription was found",
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

func (cr *Cluster) apiV0Webhook(rw http.ResponseWriter, req *http.Request) {
	if checkRequestMethodOrRejectWithJson(rw, req, http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete) {
		return
	}
	user := getLoggedUser(req)
	if user == "" {
		writeJson(rw, http.StatusForbidden, Map{
			"error": "Unauthorized",
		})
		return
	}
	switch req.Method {
	case http.MethodGet:
		cr.apiV0WebhookGET(rw, req, user)
	case http.MethodPost:
		cr.apiV0WebhookPOST(rw, req, user)
	case http.MethodPatch:
		cr.apiV0WebhookPATCH(rw, req, user)
	case http.MethodDelete:
		cr.apiV0WebhookDELETE(rw, req, user)
	default:
		panic("unreachable")
	}
}

func (cr *Cluster) apiV0WebhookGET(rw http.ResponseWriter, req *http.Request, user string) {
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

func (cr *Cluster) apiV0WebhookPOST(rw http.ResponseWriter, req *http.Request, user string) {
	data, ok := parseRequestBody[database.WebhookRecord](rw, req, nil)
	if !ok {
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

func (cr *Cluster) apiV0WebhookPATCH(rw http.ResponseWriter, req *http.Request, user string) {
	id := req.URL.Query().Get("id")
	data, ok := parseRequestBody[database.WebhookRecord](rw, req, nil)
	if !ok {
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

func (cr *Cluster) apiV0WebhookDELETE(rw http.ResponseWriter, req *http.Request, user string) {
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

type Map = map[string]any

var errUnknownContent = errors.New("unknown content-type")

type requestBodyParser[T any] func(rw http.ResponseWriter, req *http.Request, contentType string, data *T) error

func parseRequestBody[T any](rw http.ResponseWriter, req *http.Request, fallback requestBodyParser[T]) (data T, parsed bool) {
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
		if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
			writeJson(rw, http.StatusBadRequest, Map{
				"error":   "Cannot decode request body",
				"message": err.Error(),
			})
			return
		}
		return data, true
	default:
		if fallback != nil {
			if err := fallback(rw, req, contentType, &data); err == nil {
				return data, true
			} else if err != errUnknownContent {
				writeJson(rw, http.StatusBadRequest, Map{
					"error":   "Cannot decode request body",
					"message": err.Error(),
				})
				return
			}
		}
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

func checkRequestMethodOrRejectWithJson(rw http.ResponseWriter, req *http.Request, allows ...string) (rejected bool) {
	m := req.Method
	for _, a := range allows {
		if m == a {
			return false
		}
	}
	rw.Header().Set("Allow", strings.Join(allows, ", "))
	writeJson(rw, http.StatusMethodNotAllowed, Map{
		"error":  "405 method not allowed",
		"method": m,
		"allow":  allows,
	})
	return true
}
