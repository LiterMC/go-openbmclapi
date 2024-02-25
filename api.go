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
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"runtime/pprof"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

const (
	jwtIssuer = "go-openbmclapi.dashboard.api"

	clientIdCookieName = "_id"

	clientIdKey = "go-openbmclapi.cluster.client.id"
	tokenTypeKey  = "go-openbmclapi.cluster.token.typ"
	tokenIdKey  = "go-openbmclapi.cluster.token.id"
)

const (
	tokenTypeFull = "token.full"
	tokenTypeAPI = "token.api"
)

var (
	tokenIdSet    = make(map[string]struct{})
	tokenIdSetMux sync.RWMutex
)

func registerTokenId(id string) {
	tokenIdSetMux.Lock()
	defer tokenIdSetMux.Unlock()
	tokenIdSet[id] = struct{}{}
}

func unregisterTokenId(id string) {
	tokenIdSetMux.Lock()
	defer tokenIdSetMux.Unlock()
	delete(tokenIdSet, id)
}

func checkTokenId(id string) (ok bool) {
	tokenIdSetMux.RLock()
	defer tokenIdSetMux.RUnlock()
	_, ok = tokenIdSet[id]
	return
}

func apiGetClientId(req *http.Request) (id string) {
	return req.Context().Value(clientIdKey).(string)
}

func (cr *Cluster) generateAuthToken(cliId string) (string, error) {
	jti, err := genRandB64(16)
	if err != nil {
		return "", err
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"jti": jti,
		"sub": "GOBA-auth",
		"iss": jwtIssuer,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour * 24).Unix(),
		"cli": cliId,
	})
	tokenStr, err := token.SignedString(cr.apiHmacKey)
	if err != nil {
		return "", err
	}
	// registerTokenId(jti) // ignore JTI for auth token; TODO
	return tokenStr, nil
}

func (cr *Cluster) verifyAuthToken(cliId string, token string) (id string) {
	logDebugf("Authorizing %q", token)
	t, err := jwt.Parse(
		token,
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", t.Header["alg"])
			}
			return cr.apiHmacKey, nil
		},
		jwt.WithSubject("GOBA-auth"),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(jwtIssuer),
	)
	if err != nil {
		logDebugf("Cannot verity auth token: %v", err)
		return ""
	}
	c, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		logDebugf("Cannot verity auth token: claim is not jwt.MapClaims")
		return ""
	}
	if c["cli"] != cliId {
		logDebugf("Cannot verity auth token: client id not match")
		return ""
	}
	jti, ok := c["jti"].(string)
	if !ok || !checkTokenId(jti) { // ignore JTI here; TODO
		logDebugf("Cannot verity auth token: jti not exists")
		return ""
	}
	logDebugf("JTI is %s", jti)
	return jti
}

func (cr *Cluster) generateAPIToken(cliId string, path string) (string, error) {
	jti, err := genRandB64(16)
	if err != nil {
		return "", err
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"jti": jti,
		"sub": "GOBA-" + path,
		"iss": jwtIssuer,
		"iat": now.Unix(),
		"exp": now.Add(time.Minute * 10).Unix(),
		"cli": cliId,
	})
	tokenStr, err := token.SignedString(cr.apiHmacKey)
	if err != nil {
		return "", err
	}
	registerTokenId(jti)
	return tokenStr, nil
}

func (cr *Cluster) verifyAPIToken(cliId string, token string, path string) (id string) {
	logDebugf("Authorizing %q for %q", token, path)
	t, err := jwt.Parse(
		token,
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", t.Header["alg"])
			}
			return cr.apiHmacKey, nil
		},
		jwt.WithSubject("GOBA-" + path),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(jwtIssuer),
	)
	if err != nil {
		logDebugf("Cannot verity api token: %v", err)
		return ""
	}
	c, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		logDebugf("Cannot verity api token: claim is not jwt.MapClaims")
		return ""
	}
	if c["cli"] != cliId {
		logDebugf("Cannot verity api token: client id not match")
		return ""
	}
	jti, ok := c["jti"].(string)
	if !ok || !checkTokenId(jti) {
		logDebugf("Cannot verity api token: jti not exists")
		return ""
	}
	logDebugf("JTI is %s", jti)
	return jti
}

func (cr *Cluster) cliIdHandle(next http.Handler) http.Handler {
	return (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
		var id string
		if cid, _ := req.Cookie(clientIdCookieName); cid != nil {
			id = cid.Value
		} else {
			var err error
			id, err = genRandB64(16)
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
		req = req.WithContext(context.WithValue(req.Context(), clientIdKey, asSha256Hex(id)))
		next.ServeHTTP(rw, req)
	})
}

func (cr *Cluster) apiAuthHandle(next http.Handler) http.Handler {
	return (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
		if !config.Dashboard.Enable {
			writeJson(rw, http.StatusServiceUnavailable, Map{
				"error": "dashboard is disabled in the config",
			})
			return
		}
		cli := apiGetClientId(req)

		ctx := req.Context()

		var id string
		if req.Method == http.MethodGet {
			tk := req.Header.Get("_t")
			if tk != "" {
				id = cr.verifyAPIToken(cli, tk, req.URL.Path)
			}
		}
		if id == "" {
			auth := req.Header.Get("Authorization")
			tk, ok := strings.CutPrefix(auth, "Bearer ")
			if !ok {
				writeJson(rw, http.StatusUnauthorized, Map{
					"error": "invalid type of authorization",
				})
				return
			}
			id = cr.verifyAuthToken(cli, tk)
			if id == "" {
				writeJson(rw, http.StatusUnauthorized, Map{
					"error": "invalid authorization token",
				})
				return
			}
		}
		ctx = context.WithValue(ctx, tokenIdKey, id)
		req = req.WithContext(ctx)
		next.ServeHTTP(rw, req)
	})
}

func (cr *Cluster) apiAuthHandleFunc(next http.HandlerFunc) http.Handler {
	return cr.apiAuthHandle(next)
}

func (cr *Cluster) initAPIv0() http.Handler {
	wsUpgrader := websocket.Upgrader{
		HandshakeTimeout: time.Minute,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "404 not found",
			"path":  req.URL.Path,
		})
	})
	mux.HandleFunc("/ping", func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		authed := false
		cli := apiGetClientId(req)
		auth := req.Header.Get("Authorization")
		if tk, ok := strings.CutPrefix(auth, "Bearer "); ok {
			if cr.verifyAuthToken(cli, tk) != "" {
				authed = true
			}
		}
		writeJson(rw, http.StatusOK, Map{
			"version": BuildVersion,
			"time":    time.Now(),
			"authed":  authed,
		})
	})
	mux.HandleFunc("/status", func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJson(rw, http.StatusOK, Map{
			"startAt": startTime,
			"stats":   &cr.stats,
			"enabled": cr.enabled.Load(),
		})
	})
	mux.HandleFunc("/login", func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !config.Dashboard.Enable {
			writeJson(rw, http.StatusServiceUnavailable, Map{
				"error": "dashboard is disabled in the config",
			})
			return
		}
		cli := apiGetClientId(req)

		var (
			authUser, authPass string
		)
		ct, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if err != nil {
			writeJson(rw, http.StatusBadRequest, Map{
				"error":        "Unexpected Content-Type",
				"content-type": req.Header.Get("Content-Type"),
				"message":      err.Error(),
			})
			return
		}
		switch ct {
		case "application/x-www-form-urlencoded":
			authUser = req.PostFormValue("username")
			authPass = req.PostFormValue("password")
		case "application/json":
			var data struct {
				User string `json:"username"`
				Pass string `json:"password"`
			}
			if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
				writeJson(rw, http.StatusBadRequest, Map{
					"error":   "Cannot decode json body",
					"message": err.Error(),
				})
				return
			}
			authUser, authPass = data.User, data.Pass
		default:
			writeJson(rw, http.StatusBadRequest, Map{
				"error":        "Unexpected Content-Type",
				"content-type": ct,
			})
			return
		}

		expectUsername, expectPassword := config.Dashboard.Username, config.Dashboard.Password
		if expectUsername == "" || expectPassword == "" {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "The username or password was not set on the server",
			})
			return
		}
		expectPassword = asSha256Hex(expectPassword)
		if subtle.ConstantTimeCompare(([]byte)(expectUsername), ([]byte)(authUser)) == 0 ||
			subtle.ConstantTimeCompare(([]byte)(expectPassword), ([]byte)(authPass)) == 0 {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "The username or password is incorrect",
			})
			return
		}
		token, err := cr.generateAuthToken(cli)
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
	})

	mux.Handle("/requestToken", cr.apiAuthHandleFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		query := req.URL.Query()
		path := query.Get("path")
		if path == "" || path[0] != '/' {
			writeJson(rw, http.StatusBadRequest, Map{
				"error": "'path' query is invalid",
				"message": "'path' must be a non empty path which starts with '/'",
			})
			return
		}
		cli := apiGetClientId(req)
		token, err := cr.generateAPIToken(cli, path)
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
	}))

	mux.Handle("/logout", cr.apiAuthHandleFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		tid := req.Context().Value(tokenIdKey).(string)
		unregisterTokenId(tid)
	}))

	mux.HandleFunc("/log.io", func(rw http.ResponseWriter, req *http.Request) {
		addr, _ := req.Context().Value(RealAddrCtxKey).(string)

		conn, err := wsUpgrader.Upgrade(rw, req, nil)
		if err != nil {
			logDebugf("[log.io]: Websocket upgrade error: %v", err)
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()

		cli := apiGetClientId(req)

		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()

		conn.SetReadLimit(1024 * 8)
		pongCh := make(chan struct{}, 1)
		go func() {
			defer conn.Close()
			defer cancel()
			for {
				select {
				case <-pongCh:
				case <-time.After(time.Second * 75):
					logError("[log.io]: Did not receive PONG from remote for 75s")
					return
				case <-ctx.Done():
					return
				}
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
		tid := cr.verifyAuthToken(cli, authData.Token)
		if tid == "" {
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

		go func() {
			defer conn.Close()
			defer cancel()
			var data map[string]any
			for {
				clear(data)
				if err := conn.ReadJSON(&data); err != nil {
					return
				}
				typ, ok := data["type"].(string)
				if !ok {
					continue
				}
				switch typ {
				case "pong":
					logDebugf("[log.io]: received PONG from %s: %v", addr, data["data"])
					select {
					case pongCh <- struct{}{}:
					default:
					}
				}
			}
		}()

		level := LogLevelInfo
		if strings.ToLower(req.URL.Query().Get("level")) == "debug" {
			level = LogLevelDebug
		}

		type logObj struct {
			Type  string `json:"type"`
			Time  int64  `json:"time"`
			Level string `json:"lvl"`
			Log   string `json:"log"`
		}
		c := make(chan *logObj, 64)
		unregister := RegisterLogMonitor(level, func(ts int64, level LogLevel, log string) {
			select {
			case c <- &logObj{
				Type:  "log",
				Time:  ts,
				Level: level.String(),
				Log:   log,
			}:
			default:
			}
		})
		defer unregister()

		pingTimer := time.NewTicker(time.Second * 45)
		defer pingTimer.Stop()
		for {
			select {
			case v := <-c:
				if err := conn.WriteJSON(v); err != nil {
					return
				}
			case <-pingTimer.C:
				if err := conn.WriteJSON(Map{
					"type": "ping",
					"data": time.Now().UnixMilli(),
				}); err != nil {
					logErrorf("[log.io]: Error when sending ping packet: %v", err)
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
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
	return mux
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
