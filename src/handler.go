package main

import (
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	ufile "github.com/KpnmServer/go-util/file"
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
			if ufile.IsNotExist(path) {
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
			rw.Header().Set("X-Bmclapi-Hash", hash)
			rw.WriteHeader(http.StatusOK)
			var (
				buf []byte
				hb  int64
				n   int
			)
			{
				buf0 := bufPool.Get().(*[]byte)
				defer bufPool.Put(buf0)
				buf = *buf0
			}
			for {
				n, err = fd.Read(buf)
				if err != nil {
					if err == io.EOF {
						err = nil
						break
					}
					logError("Error when serving download read file:", err)
					return
				}
				if n == 0 {
					break
				}
				_, err = rw.Write(buf[:n])
				if err != nil {
					if !config.IgnoreServeError {
						logError("Error when serving download:", err)
					}
					return
				}
				hb += (int64)(n)
			}
			cr.hits.Add(1)
			cr.hbytes.Add(hb)
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
