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
)

func (h *Handler) routeSubscribeKey(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		errorMethodNotAllowed(rw, req, http.MethodGet)
		return
	}
	key := h.subManager.GetWebPushKey()
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

func (h *Handler) routeSubscribeGET(rw http.ResponseWriter, req *http.Request) {
	client := apiGetClientId(req)
	user := getLoggedUser(req)
	record, err := h.subManager.GetSubscribe(user, client)
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

func (h *Handler) routeSubscribePOST(rw http.ResponseWriter, req *http.Request) {
	client := apiGetClientId(req)
	user := getLoggedUser(req)
	data, ok := parseRequestBody[database.SubscribeRecord](rw, req, nil)
	if !ok {
		return
	}
	data.User = user
	data.Client = client
	if err := h.subManager.SetSubscribe(data); err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "Database update failed",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) routeSubscribeDELETE(rw http.ResponseWriter, req *http.Request) {
	client := apiGetClientId(req)
	user := getLoggedUser(req)
	if err := h.subManager.RemoveSubscribe(user, client); err != nil {
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

func (h *Handler) routeSubscribeEmailGET(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	if addr := req.URL.Query().Get("addr"); addr != "" {
		record, err := h.subManager.GetEmailSubscription(user, addr)
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
	if err := h.subManager.ForEachUsersEmailSubscription(user, func(rec *database.EmailSubscriptionRecord) error {
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

func (h *Handler) routeSubscribeEmailPOST(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	data, ok := parseRequestBody[database.EmailSubscriptionRecord](rw, req, nil)
	if !ok {
		return
	}

	data.User = user
	if err := h.subManager.AddEmailSubscription(data); err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "Database update failed",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusCreated)
}

func (h *Handler) routeSubscribeEmailPATCH(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	addr := req.URL.Query().Get("addr")
	data, ok := parseRequestBody[database.EmailSubscriptionRecord](rw, req, nil)
	if !ok {
		return
	}
	data.User = user
	data.Addr = addr
	if err := h.subManager.UpdateEmailSubscription(data); err != nil {
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

func (h *Handler) routeSubscribeEmailDELETE(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	addr := req.URL.Query().Get("addr")
	if err := h.subManager.RemoveEmailSubscription(user, addr); err != nil {
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
