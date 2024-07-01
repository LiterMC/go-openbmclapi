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
	"net/http"
	"time"

	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/limited"
)

func (h *Handler) buildStatRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /ping", h.routePing)
	mux.HandleFunc("GET /status", h.routeStatus)
	mux.HandleFunc("GET /stat/{name}", h.routeStat)
}

func (h *Handler) routePing(rw http.ResponseWriter, req *http.Request) {
	limited.SetSkipRateLimit(req)
	authed := getRequestTokenType(req) == tokenTypeAuth
	writeJson(rw, http.StatusOK, Map{
		"version": build.BuildVersion,
		"time":    time.Now().UnixMilli(),
		"authed":  authed,
	})
}

func (h *Handler) routeStatus(rw http.ResponseWriter, req *http.Request) {
	limited.SetSkipRateLimit(req)
	writeJson(rw, http.StatusOK, h.stats.GetStatus())
}

func (h *Handler) routeStat(rw http.ResponseWriter, req *http.Request) {
	limited.SetSkipRateLimit(req)
	name := req.PathValue("name")
	data := h.stats.GetAccessStat(name)
	if data == nil {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "AccessStatNotFoudn",
			"name":  name,
		})
		return
	}
	writeJson(rw, http.StatusOK, data)
}
