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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"runtime/pprof"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtIssuer = "go-openbmclapi.dashboard.api"

	clientIdCookieName = "_id"

	clientIdKey = "go-openbmclapi.cluster.client.id"
	tokenIdKey  = "go-openbmclapi.cluster.token.id"
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
	id, _ = req.Context().Value(clientIdKey).(string)
	return
}

func (cr *Cluster) generateToken(cliId string) (string, error) {
	jti, err := genRandB64(16)
	if err != nil {
		return "", err
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"jti": jti,
		"sub": "GOBA-tk",
		"iss": jwtIssuer,
		"iat": (int64)(now.Second()),
		"exp": (int64)(now.Add(time.Hour * 12).Second()),
		"cli": cliId,
	})
	tokenStr, err := token.SignedString(cr.apiHmacKey)
	if err != nil {
		return "", err
	}
	registerTokenId(jti)
	return tokenStr, nil
}

func (cr *Cluster) verifyAPIToken(cliId string, token string) (id string) {
	t, err := jwt.Parse(
		token,
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", t.Header["alg"])
			}
			return cr.apiHmacKey, nil
		},
		jwt.WithSubject("GOBA-tk"),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(jwtIssuer),
	)
	if err != nil {
		return ""
	}
	c, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	if c["cli"] != cliId {
		return ""
	}
	jti, ok := c["jti"].(string)
	if !ok || !checkTokenId(jti) {
		return ""
	}
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
		req = req.WithContext(context.WithValue(req.Context(), clientIdKey, id))
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
		if cli == "" {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "client id not exists",
			})
			return
		}
		auth := req.Header.Get("Authorization")
		tk, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "invalid type of authorization",
			})
			return
		}
		id := cr.verifyAPIToken(cli, tk)
		if id == "" {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "invalid authorization token",
			})
			return
		}
		req = req.WithContext(context.WithValue(req.Context(), tokenIdKey, id))
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
	mux.HandleFunc("/ping", func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJson(rw, http.StatusOK, Map{
			"version": BuildVersion,
			"time":    time.Now(),
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
		if cli == "" {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "client id not exists",
			})
			return
		}
		username, password := config.Dashboard.Username, config.Dashboard.Password
		if username == "" || password == "" {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "The username or password was not set on the server",
			})
			return
		}
		user := req.Header.Get("X-Username")
		pass := req.Header.Get("X-Password")
		if subtle.ConstantTimeCompare(([]byte)(username), ([]byte)(user)) == 0 ||
			subtle.ConstantTimeCompare(([]byte)(password), ([]byte)(pass)) == 0 {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "The username or password is incorrect",
			})
			return
		}
		token, err := cr.generateToken(cli)
		if err != nil {
			writeJson(rw, http.StatusInternalServerError, Map{
				"error": err.Error(),
			})
			return
		}
		writeJson(rw, http.StatusOK, Map{
			"token": token,
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
