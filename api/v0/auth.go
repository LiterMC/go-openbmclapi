/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2024 Kevin Z <zyxkad@gmail.com>
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
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/utils"
)

const (
	clientIdCookieName = "_id"

	clientIdKey = "go-openbmclapi.cluster.client.id"
)

func apiGetClientId(req *http.Request) (id string) {
	return req.Context().Value(clientIdKey).(string)
}

const jwtIssuerPrefix = "GOBA.dash.api"

const (
	tokenTypeKey = "go-openbmclapi.cluster.token.typ"
	tokenIdKey   = "go-openbmclapi.cluster.token.id"

	loggedUserKey = "go-openbmclapi.cluster.logged.user"
)

const (
	tokenTypeAuth = "auth"
	tokenTypeAPI  = "api"
)

func getRequestTokenType(req *http.Request) string {
	if t, ok := req.Context().Value(tokenTypeKey).(string); ok {
		return t
	}
	return ""
}

func getLoggedUser(req *http.Request) *api.User {
	if user, ok := req.Context().Value(loggedUserKey).(*api.User); ok {
		return user
	}
	return nil
}

func cliIdMiddleWare(rw http.ResponseWriter, req *http.Request, next http.Handler) {
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
}

func (h *Handler) authMiddleWare(rw http.ResponseWriter, req *http.Request, next http.Handler) {
	cli := apiGetClientId(req)

	ctx := req.Context()

	var (
		typ string
		id  string
		uid string
		err error
	)
	if req.Method == http.MethodGet {
		if tk := req.URL.Query().Get("_t"); tk != "" {
			path := api.GetRequestRealPath(req)
			if uid, err = h.tokens.VerifyAPIToken(cli, tk, path, req.URL.Query()); err == nil {
				typ = tokenTypeAPI
			}
		}
	}
	if typ == "" {
		auth := req.Header.Get("Authorization")
		tk, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok {
			if err == nil {
				err = errors.New("Unsupported authorization type")
			}
		} else if id, uid, err = h.tokens.VerifyAuthToken(cli, tk); err == nil {
			typ = tokenTypeAuth
		}
	}
	if typ != "" {
		user := h.users.GetUser(uid)
		if user != nil {
			ctx = context.WithValue(ctx, tokenTypeKey, typ)
			ctx = context.WithValue(ctx, loggedUserKey, user)
			ctx = context.WithValue(ctx, tokenIdKey, id)
			req = req.WithContext(ctx)
		}
	}
	next.ServeHTTP(rw, req)
}

func authHandle(next http.Handler) http.Handler {
	return permHandle(api.BasicPerm, next)
}

func authHandleFunc(next http.HandlerFunc) http.Handler {
	return authHandle(next)
}

func permHandle(perm api.PermissionFlag, next http.Handler) http.Handler {
	perm |= api.BasicPerm
	return (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
		user := getLoggedUser(req)
		if user == nil {
			writeJson(rw, http.StatusUnauthorized, Map{
				"error": "403 Unauthorized",
			})
			return
		}
		if user.Permissions&perm != perm {
			writeJson(rw, http.StatusForbidden, Map{
				"error": "Permission denied",
			})
			return
		}
		next.ServeHTTP(rw, req)
	})
}

func permHandleFunc(perm api.PermissionFlag, next http.HandlerFunc) http.Handler {
	return permHandle(perm, next)
}

func (h *Handler) buildAuthRoute(mux *http.ServeMux) {
	mux.HandleFunc("/challenge", h.routeChallenge)
	mux.HandleFunc("POST /login", h.routeLogin)
	mux.Handle("POST /requestToken", authHandleFunc(h.routeRequestToken))
	mux.Handle("POST /logout", authHandleFunc(h.routeLogout))
}

func (h *Handler) routeChallenge(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		errorMethodNotAllowed(rw, req, http.MethodGet)
		return
	}
	cli := apiGetClientId(req)
	query := req.URL.Query()
	action := query.Get("action")
	token, err := h.tokens.GenerateChallengeToken(cli, action)
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
	cli := apiGetClientId(req)

	var data struct {
		User      string `json:"username" schema:"username"`
		Challenge string `json:"challenge" schema:"challenge"`
		Signature string `json:"signature" schema:"signature"`
	}
	if !parseRequestBody(rw, req, &data) {
		return
	}

	if err := h.tokens.VerifyChallengeToken(cli, data.Challenge, "login"); err != nil {
		writeJson(rw, http.StatusUnauthorized, Map{
			"error": "Invalid challenge",
		})
		return
	}
	if err := h.users.VerifyUserPassword(data.User, func(password string) bool {
		expectSignature := utils.HMACSha256HexBytes(password, data.Challenge)
		return subtle.ConstantTimeCompare(expectSignature, ([]byte)(data.Signature)) == 0
	}); err != nil {
		writeJson(rw, http.StatusUnauthorized, Map{
			"error": "The username or password is incorrect",
		})
		return
	}
	token, err := h.tokens.GenerateAuthToken(cli, data.User)
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

func (h *Handler) routeRequestToken(rw http.ResponseWriter, req *http.Request) {
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
	token, err := h.tokens.GenerateAPIToken(cli, user.Username, payload.Path, payload.Query)
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

func (h *Handler) routeLogout(rw http.ResponseWriter, req *http.Request) {
	limited.SetSkipRateLimit(req)
	tid := req.Context().Value(tokenIdKey).(string)
	h.tokens.InvalidToken(tid)
	rw.WriteHeader(http.StatusNoContent)
}
