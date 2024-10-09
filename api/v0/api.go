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
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"

	"github.com/gorilla/schema"
	"github.com/gorilla/websocket"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type Handler struct {
	handler       *utils.HttpMiddleWareHandler
	router        *http.ServeMux
	wsUpgrader    *websocket.Upgrader
	config        api.ConfigHandler
	users         api.UserManager
	tokens        api.TokenManager
	subscriptions api.SubscriptionManager
	stats         api.StatsManager
}

var _ http.Handler = (*Handler)(nil)

func NewHandler(
	wsUpgrader *websocket.Upgrader,
	config api.ConfigHandler,
	users api.UserManager,
	tokenManager api.TokenManager,
	subManager api.SubscriptionManager,
) *Handler {
	mux := http.NewServeMux()
	h := &Handler{
		router:        mux,
		handler:       utils.NewHttpMiddleWareHandler(mux),
		wsUpgrader:    wsUpgrader,
		config:        config,
		users:         users,
		tokens:        tokenManager,
		subscriptions: subManager,
	}
	h.buildRoute()
	h.handler.UseFunc(cliIdMiddleWare, h.authMiddleWare)
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

	h.buildStatRoute(mux)
	h.buildAuthRoute(mux)
	h.buildSubscriptionRoute(mux)
	h.buildConfigureRoute(mux)
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.handler.ServeHTTP(rw, req)
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
}
