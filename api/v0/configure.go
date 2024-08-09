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
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/LiterMC/go-openbmclapi/api"
)

func (h *Handler) buildConfigureRoute(mux *http.ServeMux) {
	mux.Handle("GET /config", permHandleFunc(api.FullConfigPerm, h.routeConfigGET))
	mux.Handle("GET /config/{path}", permHandleFunc(api.FullConfigPerm, h.routeConfigGETPath))
	mux.Handle("PUT /config", permHandleFunc(api.FullConfigPerm, h.routeConfigPUT))
	mux.Handle("PATCH /config/{path}", permHandleFunc(api.FullConfigPerm, h.routeConfigPATCH))
	mux.Handle("DELETE /config/{path}", permHandleFunc(api.FullConfigPerm, h.routeConfigDELETE))

	mux.Handle("GET /configure/clusters", permHandleFunc(api.ClusterPerm, h.routeConfigureClustersGET))
	mux.Handle("GET /configure/cluster/{cluster_id}", permHandleFunc(api.ClusterPerm, h.routeConfigureClusterGET))
	mux.Handle("PUT /configure/cluster/{cluster_id}", permHandleFunc(api.ClusterPerm, h.routeConfigureClusterPUT))
	mux.Handle("PATCH /configure/cluster/{cluster_id}/{path}", permHandleFunc(api.ClusterPerm, h.routeConfigureClusterPATCH))
	mux.Handle("DELETE /configure/cluster/{cluster_id}", permHandleFunc(api.ClusterPerm, h.routeConfigureClusterDELETE))

	mux.Handle("GET /configure/storages", permHandleFunc(api.StoragePerm, h.routeConfigureStoragesGET))
	mux.Handle("GET /configure/storage/{storage_index}", permHandleFunc(api.StoragePerm, h.routeConfigureStorageGET))
	mux.Handle("PUT /configure/storage/{storage_index}", permHandleFunc(api.StoragePerm, h.routeConfigureStoragePUT))
	mux.Handle("PATCH /configure/storage/{storage_index}/{path}", permHandleFunc(api.StoragePerm, h.routeConfigureStoragePATCH))
	mux.Handle("DELETE /configure/storage/{storage_index}", permHandleFunc(api.StoragePerm, h.routeConfigureStorageDELETE))
	mux.Handle("POST /configure/storage/{storage_index}/move", permHandleFunc(api.StoragePerm, h.routeConfigureStorageMove))
}

func (h *Handler) routeConfigGET(rw http.ResponseWriter, req *http.Request) {
	buf, err := h.config.MarshalJSON()
	if err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "MarshalJSONError",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusOK)
	rw.Write(buf)
}

func (h *Handler) routeConfigPUT(rw http.ResponseWriter, req *http.Request) {
	contentType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":        "Unexpected Content-Type",
			"content-type": req.Header.Get("Content-Type"),
			"message":      err.Error(),
		})
		return
	}
	etag := req.Header.Get("If-Match")
	if len(etag) > 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		etag = etag[1 : len(etag)-1]
	} else {
		etag = ""
	}
	err = h.config.DoLockedAction(etag, func(config api.ConfigHandler) error {
		switch contentType {
		case "application/json":
			buf, err := io.ReadAll(req.Body)
			if err != nil {
				return fmt.Errorf("Failed to read request body: %w", err)
			}
			return config.UnmarshalJSON(buf)
		case "application/x-yaml":
			buf, err := io.ReadAll(req.Body)
			if err != nil {
				return fmt.Errorf("Failed to read request body: %w", err)
			}
			return config.UnmarshalYAML(buf)
		default:
			return errUnknownContent
		}
	})
	if err != nil {
		if err == errUnknownContent {
			writeJson(rw, http.StatusBadRequest, Map{
				"error":        "Unexpected Content-Type",
				"content-type": req.Header.Get("Content-Type"),
				"message":      "Expected application/json, application/x-yaml",
			})
			return
		}
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "UnmarshalError",
			"message": err.Error(),
		})
		return
	}
}

func (h *Handler) routeConfigGETPath(rw http.ResponseWriter, req *http.Request) {
	rw.WriteHeader(http.StatusOK)
}

func (h *Handler) routeConfigPATCH(rw http.ResponseWriter, req *http.Request) {
}

func (h *Handler) routeConfigDELETE(rw http.ResponseWriter, req *http.Request) {
}

func (h *Handler) routeConfigureClustersGET(rw http.ResponseWriter, req *http.Request) {
	//
}

func (h *Handler) routeConfigureClusterGET(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.PathValue("cluster_id")
	_ = clusterId
}

func (h *Handler) routeConfigureClusterPUT(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.PathValue("cluster_id")
	_ = clusterId
}

func (h *Handler) routeConfigureClusterPATCH(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.PathValue("cluster_id")
	path := req.PathValue("path")
	_, _ = clusterId, path
}

func (h *Handler) routeConfigureClusterDELETE(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.PathValue("cluster_id")
	_ = clusterId
}

func (h *Handler) routeConfigureStoragesGET(rw http.ResponseWriter, req *http.Request) {
}

func (h *Handler) routeConfigureStorageGET(rw http.ResponseWriter, req *http.Request) {
	storageIndex := req.PathValue("storage_index")
	_ = storageIndex
}

func (h *Handler) routeConfigureStoragePUT(rw http.ResponseWriter, req *http.Request) {
	storageIndex := req.PathValue("storage_index")
	_ = storageIndex
}

func (h *Handler) routeConfigureStoragePATCH(rw http.ResponseWriter, req *http.Request) {
	storageIndex := req.PathValue("storage_index")
	path := req.PathValue("path")
	_, _ = storageIndex, path
}

func (h *Handler) routeConfigureStorageDELETE(rw http.ResponseWriter, req *http.Request) {
	storageIndex := req.PathValue("storage_index")
	_ = storageIndex
}

func (h *Handler) routeConfigureStorageMove(rw http.ResponseWriter, req *http.Request) {
	storageIndex := req.PathValue("storage_index")
	storageIndexTo := req.URL.Query().Get("to")
	_, _ = storageIndex, storageIndexTo
}
