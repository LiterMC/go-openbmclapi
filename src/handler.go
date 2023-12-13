package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const maxZeroBufMB = 200

var zeroBuffer [maxZeroBufMB * 1024 * 1024]byte

func (cr *Cluster) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	method := req.Method
	url := req.URL
	rawpath := url.EscapedPath()
	if config.ShowServeInfo {
		logInfo("serve url:", url.String())
	}
	switch {
	case strings.HasPrefix(rawpath, "/download/"):
		if method == http.MethodGet {
			hash := rawpath[len("/download/"):]
			path := cr.getHashPath(hash)
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				if err := cr.DownloadFile(req.Context(), path); err != nil {
					rw.WriteHeader(http.StatusInternalServerError)
					return
				}
			}
			if name := req.Form.Get("name"); name != "" {
				rw.Header().Set("Content-Disposition", "attachment; filename="+name)
			}
			rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
			fd, err := os.Open(path)
			if err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer fd.Close()
			rw.Header().Set("X-Bmclapi-Hash", hash)
			rw.WriteHeader(http.StatusOK)
			var buf []byte
			{
				buf0 := bufPool.Get().(*[]byte)
				defer bufPool.Put(buf0)
				buf = *buf0
			}
			n, err := io.CopyBuffer(rw, fd, buf)
			cr.hits.Add(1)
			cr.hbytes.Add(n)
			if err != nil {
				if !config.IgnoreServeError {
					logError("Error when serving download:", err)
				}
				return
			}
			return
		}
	case strings.HasPrefix(rawpath, "/measure/"):
		if method == http.MethodGet {
			if req.Header.Get("x-openbmclapi-secret") != cr.password {
				rw.WriteHeader(http.StatusForbidden)
				return
			}
			n, e := strconv.Atoi(rawpath[len("/measure/"):])
			if e != nil || n < 0 || n > maxZeroBufMB {
				rw.WriteHeader(http.StatusBadRequest)
				return
			}
			rw.WriteHeader(http.StatusOK)
			rw.Write(zeroBuffer[:n*1024*1024])
			return
		}
	}
	rw.WriteHeader(http.StatusNotFound)
	rw.Write(([]byte)("404 Status Not Found"))
}
