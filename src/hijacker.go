package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func getDialerWithDNS(dnsaddr string) *net.Dialer {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return net.Dial(network, dnsaddr)
		},
	}
	return &net.Dialer{
		Resolver: resolver,
	}
}

type HjProxy struct {
	dialer *net.Dialer
	path   string
	client *http.Client
}

func NewHjProxy(dialer *net.Dialer, path string) (h *HjProxy) {
	return &HjProxy{
		dialer: dialer,
		path:   path,
		client: &http.Client{
			Timeout: time.Second * 15,
			Transport: &http.Transport{
				DialContext: dialer.DialContext,
			},
		},
	}
}

const hijackingHost = "bmclapi2.bangbang93.com"

func (h *HjProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		target := filepath.Join(h.path, filepath.Clean(filepath.FromSlash(req.URL.Path)))
		fd, err := os.Open(target)
		if err == nil {
			defer fd.Close()
			if stat, err := fd.Stat(); err == nil {
				if !stat.IsDir() {
					modTime := stat.ModTime()
					http.ServeContent(rw, req, filepath.Base(target), modTime, fd)
					return
				}
			}
		}
	}

	u := *req.URL
	u.Scheme = "https"
	u.Host = hijackingHost
	req2, err := http.NewRequestWithContext(req.Context(), req.Method, u.String(), req.Body)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	res, err := h.client.Do(req2)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	for k, v := range res.Header {
		rw.Header()[k] = v
	}
	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
}
