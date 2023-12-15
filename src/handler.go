package main

import (
	"errors"
	"io"
	"net"
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
	cr.handlerAPIv0 = http.StripPrefix("/api/v0", cr.initAPIv0())

	handler = cr
	{
		totalUsedCh := make(chan float64, 1024)

		next := handler
		handler = (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
			srw := &statusResponseWriter{ResponseWriter: rw}
			start := time.Now()

			next.ServeHTTP(srw, req)

			used := time.Since(start)
			if config.RecordServeInfo {
				addr, _, _ := net.SplitHostPort(req.RemoteAddr)
				logInfof("Serve %d | %6v | %-15s | %s | %-4s %s", srw.status, used, addr, req.Proto, req.Method, req.RequestURI)
			}
			totalUsedCh <- used.Seconds()
		})
		go func(disabled <-chan struct{}) {
			var (
				total     int64
				totalUsed float64
			)
			for {
				select {
				case used := <-totalUsedCh:
					totalUsed += used
					total++
					if total%100 == 0 {
						avg := (time.Duration)(totalUsed / (float64)(total) * (float64)(time.Second))
						logInfof("Served %d requests, total used %.2fs, avg %v", total, totalUsed, avg)
					}
				case <-disabled:
					return
				}
			}
		}(cr.Disabled())
	}
	return
}

func (cr *Cluster) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	method := req.Method
	u := req.URL
	rawpath := u.EscapedPath()
	switch {
	case strings.HasPrefix(rawpath, "/download/"):
		if method != http.MethodGet && method != http.MethodHead {
			rw.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
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
			target, err := url.JoinPath(cr.redirectBase, "download", hashToFilename(hash))
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
		if method != http.MethodGet && method != http.MethodHead {
			rw.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
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
		if method == http.MethodGet {
			for i := 0; i < n; i++ {
				rw.Write(zeroBuffer[:])
			}
		}
		return
	case strings.HasPrefix(rawpath, "/api/"):
		version, _ := split(rawpath[len("/api/"):], '/')
		switch version {
		case "v0":
			cr.handlerAPIv0.ServeHTTP(rw, req)
			return
		}
	case strings.HasPrefix(rawpath, "/dashboard/"):
		pth := rawpath[len("/dashboard/"):]
		cr.serveDashboard(rw, req, pth)
		return
	case rawpath == "/" || rawpath == "/dashboard":
		http.Redirect(rw, req, "/dashboard/", http.StatusFound)
		return
	}
	http.NotFound(rw, req)
}
