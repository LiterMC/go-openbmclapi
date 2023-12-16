package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (cr *Cluster) initAPIv0() (mux *http.ServeMux) {
	mux = http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusNotFound, Map{
			"error": "404 not found",
			"path":  req.URL.Path,
		})
	})
	mux.HandleFunc("/ping", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusOK, Map{
			"version": BuildVersion,
			"time":    time.Now(),
		})
	})
	mux.HandleFunc("/status", func(rw http.ResponseWriter, req *http.Request) {
		writeJson(rw, http.StatusOK, Map{
			"startAt": startTime,
			"stats":   cr.stats,
			"enabled": cr.enabled.Load(),
		})
	})
	return
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
