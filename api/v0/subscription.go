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

	"github.com/google/uuid"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/utils"
)

func (h *Handler) buildSubscriptionRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /subscribeKey", h.routeSubscribeKey)
	mux.Handle("/subscribe", permHandle(api.SubscribePerm, &utils.HttpMethodHandler{
		Get:    (http.HandlerFunc)(h.routeSubscribeGET),
		Post:   (http.HandlerFunc)(h.routeSubscribePOST),
		Delete: (http.HandlerFunc)(h.routeSubscribeDELETE),
	}))
	mux.Handle("/subscribe_email", permHandle(api.SubscribePerm, &utils.HttpMethodHandler{
		Get:    (http.HandlerFunc)(h.routeSubscribeEmailGET),
		Post:   (http.HandlerFunc)(h.routeSubscribeEmailPOST),
		Patch:  (http.HandlerFunc)(h.routeSubscribeEmailPATCH),
		Delete: (http.HandlerFunc)(h.routeSubscribeEmailDELETE),
	}))
	mux.Handle("/webhook", permHandle(api.SubscribePerm, &utils.HttpMethodHandler{
		Get:    (http.HandlerFunc)(h.routeWebhookGET),
		Post:   (http.HandlerFunc)(h.routeWebhookPOST),
		Patch:  (http.HandlerFunc)(h.routeWebhookPATCH),
		Delete: (http.HandlerFunc)(h.routeWebhookDELETE),
	}))
}

func (h *Handler) routeSubscribeKey(rw http.ResponseWriter, req *http.Request) {
	key := h.subscriptions.GetWebPushKey()
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
	record, err := h.subscriptions.GetSubscribe(user.Username, client)
	if err != nil {
		if err == api.ErrNotFound {
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
	var data api.SubscribeRecord
	if !parseRequestBody(rw, req, &data) {
		return
	}
	data.User = user.Username
	data.Client = client
	if err := h.subscriptions.SetSubscribe(data); err != nil {
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
	if err := h.subscriptions.RemoveSubscribe(user.Username, client); err != nil {
		if err == api.ErrNotFound {
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
		record, err := h.subscriptions.GetEmailSubscription(user.Username, addr)
		if err != nil {
			if err == api.ErrNotFound {
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
	records := make([]api.EmailSubscriptionRecord, 0, 4)
	if err := h.subscriptions.ForEachUsersEmailSubscription(user.Username, func(rec *api.EmailSubscriptionRecord) error {
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
	var data api.EmailSubscriptionRecord
	if !parseRequestBody(rw, req, &data) {
		return
	}

	data.User = user.Username
	if err := h.subscriptions.AddEmailSubscription(data); err != nil {
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
	var data api.EmailSubscriptionRecord
	if !parseRequestBody(rw, req, &data) {
		return
	}
	data.User = user.Username
	data.Addr = addr
	if err := h.subscriptions.UpdateEmailSubscription(data); err != nil {
		if err == api.ErrNotFound {
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
	if err := h.subscriptions.RemoveEmailSubscription(user.Username, addr); err != nil {
		if err == api.ErrNotFound {
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

func (h *Handler) routeWebhookGET(rw http.ResponseWriter, req *http.Request) {
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
		record, err := h.subscriptions.GetWebhook(user.Username, id)
		if err != nil {
			if err == api.ErrNotFound {
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
	records := make([]api.WebhookRecord, 0, 4)
	if err := h.subscriptions.ForEachUsersWebhook(user.Username, func(rec *api.WebhookRecord) error {
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

func (h *Handler) routeWebhookPOST(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	var data api.WebhookRecord
	if !parseRequestBody(rw, req, &data) {
		return
	}

	data.User = user.Username
	if err := h.subscriptions.AddWebhook(data); err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "Database update failed",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusCreated)
}

func (h *Handler) routeWebhookPATCH(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	id := req.URL.Query().Get("id")
	var data api.WebhookRecord
	if !parseRequestBody(rw, req, &data) {
		return
	}
	data.User = user.Username
	var err error
	if data.Id, err = uuid.Parse(id); err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "uuid format error",
			"message": err.Error(),
		})
		return
	}
	if err := h.subscriptions.UpdateWebhook(data); err != nil {
		if err == api.ErrNotFound {
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

func (h *Handler) routeWebhookDELETE(rw http.ResponseWriter, req *http.Request) {
	user := getLoggedUser(req)
	id, err := uuid.Parse(req.URL.Query().Get("id"))
	if err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "uuid format error",
			"message": err.Error(),
		})
		return
	}
	if err := h.subscriptions.RemoveWebhook(user.Username, id); err != nil {
		if err == api.ErrNotFound {
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
