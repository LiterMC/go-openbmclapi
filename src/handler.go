package main

import (
	"errors"
	"io"
	"net/http"
	"net/url"
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

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (cr *Cluster) GetHandler() (handler http.Handler) {
	handler = cr
	if config.RecordServeInfo {
		totalUsedCh := make(chan float64, 1024)

		next := handler
		handler = (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
			srw := &statusResponseWriter{ResponseWriter: rw}
			start := time.Now()

			next.ServeHTTP(srw, req)

			used := time.Since(start)
			logInfof("Serve %s | %d | %s | %s %s | %v", req.RemoteAddr, srw.status, req.Proto, req.Method, req.RequestURI, used)
			totalUsedCh <- used.Seconds()
		})
		go func() {
			var (
				total int64
				totalUsed float64
			)
			for {
				select{
				case used := <-totalUsedCh:
					totalUsed += used
					total++
					if total % 100 == 0 {
						avg := (time.Duration)(totalUsed / (float64)(total) * (float64)(time.Second))
						logInfof("Served %d requests, total used %.2fs, avg %v", total, totalUsed, avg)
					}
				case <-cr.disabled:
					return
				}
			}
		}()
	}
	return
}

func (cr *Cluster) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	method := req.Method
	u := req.URL
	rawpath := u.EscapedPath()
	if method != http.MethodGet && method != http.MethodHead {
		http.Error(rw, "404 Status Not Found", http.StatusNotFound)
		return
	}
	switch {
	case strings.HasPrefix(rawpath, "/download/"):
		hash := rawpath[len("/download/"):]
		if len(hash) < 4 {
			http.Error(rw, "404 Status Not Found", http.StatusNotFound)
			return
		}
		path := cr.getCachedHashPath(hash)
		stat, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			if err := cr.DownloadFile(req.Context(), path); err != nil {
				http.Error(rw, "404 Status Not Found", http.StatusNotFound)
				return
			}
		}
		name := req.Form.Get("name")
		if cr.redirectBase != "" {
			target, err := url.JoinPath(cr.redirectBase, "download", cr.joinHashPath(hash))
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(rw, req, target, http.StatusFound)
			cr.hits.Add(1)
			cr.hbts.Add(stat.Size())
			return
		}
		rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		fd, err := os.Open(path)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		defer fd.Close()
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Header().Set("X-Bmclapi-Hash", hash)
		counter := &countReader{ReadSeeker: fd}
		http.ServeContent(rw, req, name, time.Time{}, counter)
		cr.hits.Add(1)
		cr.hbts.Add(counter.n)
		return
	case strings.HasPrefix(rawpath, "/measure/"):
		if req.Header.Get("x-openbmclapi-secret") != cr.password {
			rw.WriteHeader(http.StatusForbidden)
			return
		}
		if cr.redirectBase != "" {
			target, err := url.JoinPath(cr.redirectBase, rawpath)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(rw, req, target, http.StatusFound)
			return
		}
		n, e := strconv.Atoi(rawpath[len("/measure/"):])
		if e != nil || n < 0 || n > 200 {
			http.Error(rw, e.Error(), http.StatusBadRequest)
			return
		}
		rw.Header().Set("Content-Length", strconv.Itoa(n*len(zeroBuffer)))
		rw.WriteHeader(http.StatusOK)
		for i := 0; i < n; i++ {
			rw.Write(zeroBuffer[:])
		}
		return
	}
	http.Error(rw, "404 Status Not Found", http.StatusNotFound)
}
