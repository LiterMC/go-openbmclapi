/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2025 Kevin Z <zyxkad@gmail.com>
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
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/config"
	"github.com/LiterMC/go-openbmclapi/log"
)

func (h *Handler) buildClusterRoute(mux *http.ServeMux) {
	mux.Handle("GET /cluster/list", permHandleFunc(api.ClusterPerm, h.routeClusterList))
	mux.Handle("GET /cluster/status", permHandleFunc(api.ClusterPerm, h.routeClusterStatus))
	mux.Handle("GET /cluster/config", permHandleFunc(api.ClusterPerm, h.routeClusterConfigGET))
	mux.Handle("PUT /cluster/config", permHandleFunc(api.ClusterPerm, h.routeClusterConfigPUT))
	mux.Handle("DELETE /cluster/config", permHandleFunc(api.ClusterPerm, h.routeClusterConfigDELETE))
	mux.Handle("POST /cluster/connect", permHandleFunc(api.ClusterPerm, h.routeClusterConnect))
	mux.Handle("POST /cluster/sync", permHandleFunc(api.ClusterPerm, h.routeClusterSync))
	mux.Handle("POST /cluster/enable", permHandleFunc(api.ClusterPerm, h.routeClusterEnable))
	mux.Handle("POST /cluster/disable", permHandleFunc(api.ClusterPerm, h.routeClusterDisable))
}

func (h *Handler) routeClusterList(rw http.ResponseWriter, req *http.Request) {
	data, err := json.Marshal(h.config.GetConfig().Clusters)
	if err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "MarshalJSONError",
			"message": err.Error(),
		})
		return
	}
	etag := calcSha256ETag(data)
	rw.Header().Set("ETag", etag)
	if req.Header.Get("If-None-Match") == etag {
		rw.WriteHeader(http.StatusNotModified)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Content-Length", strconv.Itoa(len(data)))
	rw.WriteHeader(http.StatusOK)
	rw.Write(data)
}

func (h *Handler) routeClusterStatus(rw http.ResponseWriter, req *http.Request) {
	type clusterStatus struct {
		Status api.ClusterStatus `json:"status"`
		Sync   bool              `json:"sync"`
	}
	data := make(map[string]*clusterStatus)
	for _, cluster := range h.clusters.GetClusters() {
		data[cluster.Name()] = &clusterStatus{
			Status: cluster.Status(),
			Sync:   false, // TODO
		}
	}
	writeJson(rw, http.StatusOK, data)
}

func (h *Handler) routeClusterConfigGET(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.URL.Query().Get("cluster_id")
	clusterCfg, ok := h.config.GetConfig().Clusters[clusterId]
	if !ok {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "ClusterNotFound",
		})
		return
	}
	data, err := json.Marshal(clusterCfg)
	if err != nil {
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "MarshalJSONError",
			"message": err.Error(),
		})
		return
	}
	etag := calcSha256ETag(data)
	rw.Header().Set("ETag", etag)
	if req.Header.Get("If-None-Match") == etag {
		rw.WriteHeader(http.StatusNotModified)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Content-Length", strconv.Itoa(len(data)))
	rw.WriteHeader(http.StatusOK)
	rw.Write(data)
}

func (h *Handler) routeClusterConfigPUT(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.URL.Query().Get("cluster_id")
	contentType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		writeJson(rw, http.StatusUnsupportedMediaType, Map{
			"error":        "Unexpected Content-Type",
			"content-type": req.Header.Get("Content-Type"),
			"message":      err.Error(),
		})
		return
	}
	etag := req.Header.Get("If-Match")
	if err := h.config.DoWriteLockedAction(func(cfg api.ConfigHandler) error {
		clustersCfg := cfg.GetConfig().Clusters
		if etag != "" {
			ccfg, ok := clustersCfg[clusterId]
			if !ok {
				return api.ErrPreconditionFailed
			}
			if data, err := json.Marshal(ccfg); err != nil {
				return err
			} else if etag != calcSha256ETag(data) {
				return api.ErrPreconditionFailed
			}
		}
		var clusterCfg config.ClusterOptions
		switch contentType {
		case "application/json":
			buf, err := io.ReadAll(req.Body)
			if err != nil {
				return fmt.Errorf("Failed to read request body: %w", err)
			}
			if err := json.Unmarshal(buf, &clusterCfg); err != nil {
				return err
			}
		default:
			return errUnknownContent
		}
		clustersCfg[clusterId] = clusterCfg
		return nil
	}); err != nil {
		if err == api.ErrPreconditionFailed {
			rw.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		if err == errUnknownContent {
			writeJson(rw, http.StatusUnsupportedMediaType, Map{
				"error":        "Unexpected Content-Type",
				"content-type": req.Header.Get("Content-Type"),
				"message":      "Expected application/json",
			})
			return
		}
		writeJson(rw, http.StatusBadRequest, Map{
			"error":   "UnmarshalError",
			"message": err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) routeClusterConfigDELETE(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.URL.Query().Get("cluster_id")
	etag := req.Header.Get("If-Match")
	if err := h.config.DoWriteLockedAction(func(config api.ConfigHandler) error {
		clustersCfg := h.config.GetConfig().Clusters
		if etag != "" {
			buf, err := json.Marshal(clustersCfg)
			if err != nil {
				return err
			}
			if etag != calcSha256ETag(buf) {
				return api.ErrPreconditionFailed
			}
		}
		_, ok := clustersCfg[clusterId]
		if !ok {
			return api.ErrPreconditionFailed
		}
		delete(clustersCfg, clusterId)
		return nil
	}); err != nil {
		if err == api.ErrPreconditionFailed {
			rw.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "InternalServerError",
			"message": err.Error(),
		})
		return
	}
	cluster := h.clusters.GetCluster(clusterId)
	if cluster == nil {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "ClusterNotFound",
		})
		return
	}
	go func() {
		if err := cluster.Disable(context.Background()); err != nil {
			log.Errorf("API Disable Error: %v", err)
		}
		cluster.Disconnect(context.Background())
	}()
	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) routeClusterConnect(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.URL.Query().Get("cluster_id")
	cluster := h.clusters.GetCluster(clusterId)
	if cluster == nil {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "ClusterNotFound",
		})
		return
	}
	go func() {
		err := cluster.Connect(context.Background())
		if err != nil {
			log.Errorf("API Connect Error: %v", err)
		}
	}()
	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) routeClusterSync(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.URL.Query().Get("cluster_id")
	clu := h.clusters.GetCluster(clusterId)
	fileMap := make(map[string]*api.StorageFileInfo)
	if err := clu.GetFileList(req.Context(), fileMap, false); err != nil {
		writeJson(rw, http.StatusInternalServerError, Map{
			"error":   "FileListFetchError",
			"message": err.Error(),
		})
		return
	}
	go func() {
		// TODO: sync file
		// need make sure no conflict with timed sync
	}()
	writeJson(rw, http.StatusOK, Map{
		"count": len(fileMap),
	})
}

func (h *Handler) routeClusterEnable(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.URL.Query().Get("cluster_id")
	cluster := h.clusters.GetCluster(clusterId)
	if cluster == nil {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "ClusterNotFound",
		})
		return
	}
	go func() {
		err := cluster.Enable(context.Background())
		if err != nil {
			log.Errorf("API Enable Error: %v", err)
		}
	}()
	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) routeClusterDisable(rw http.ResponseWriter, req *http.Request) {
	clusterId := req.URL.Query().Get("cluster_id")
	cluster := h.clusters.GetCluster(clusterId)
	if cluster == nil {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "ClusterNotFound",
		})
		return
	}
	go func() {
		err := cluster.Disable(context.Background())
		if err != nil {
			log.Errorf("API Disable Error: %v", err)
		}
		cluster.Disconnect(context.Background())
	}()
	rw.WriteHeader(http.StatusNoContent)
}
