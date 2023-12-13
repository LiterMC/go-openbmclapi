package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var zeroBuffer [1024 * 1024]byte

type countReader struct {
	io.ReadSeeker
	n int64
}

func (r *countReader) Read(buf []byte) (n int, err error) {
	n, err = r.ReadSeeker.Read(buf)
	r.n += (int64)(n)
	return
}

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
			name := req.Form.Get("name")
			rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
			fd, err := os.Open(path)
			if err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer fd.Close()
			rw.Header().Set("X-Bmclapi-Hash", hash)
			rw.WriteHeader(http.StatusOK)
			counter := &countReader{ReadSeeker: fd}
			http.ServeContent(rw, req, name, time.Time{}, counter)
			cr.hits.Add(1)
			cr.hbytes.Add(counter.n)
			return
		}
	case strings.HasPrefix(rawpath, "/measure/"):
		if method == http.MethodGet {
			if req.Header.Get("x-openbmclapi-secret") != cr.password {
				rw.WriteHeader(http.StatusForbidden)
				return
			}
			n, e := strconv.Atoi(rawpath[len("/measure/"):])
			if e != nil || n < 0 || n > 200 {
				rw.WriteHeader(http.StatusBadRequest)
				return
			}
			rw.Header().Set("Content-Length", strconv.Itoa(n * len(zeroBuffer)))
			rw.WriteHeader(http.StatusOK)
			for i := 0; i < n; i++ {
				rw.Write(zeroBuffer[:])
			}
			return
		}
	}
	rw.WriteHeader(http.StatusNotFound)
	rw.Write(([]byte)("404 Status Not Found"))
}
