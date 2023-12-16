package main

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:generate npm -C dashboard ci
//go:generate npm -C dashboard run build

//go:embed dashboard/dist
var _dsbDist embed.FS
var dsbDist = func() fs.FS {
	s, e := fs.Sub(_dsbDist, "dashboard/dist")
	if e != nil {
		panic(e)
	}
	return s
}()

//go:embed dashboard/dist/index.html
var dsbIndexHtml string

func (cr *Cluster) serveDashboard(rw http.ResponseWriter, req *http.Request, pth string) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		rw.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		http.Error(rw, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if pth != "" {
		fd, err := dsbDist.Open(pth)
		if err == nil {
			defer fd.Close()
			if stat, err := fd.Stat(); err != nil || stat.IsDir() {
				http.NotFound(rw, req)
				return
			}
			name := path.Base(pth)
			typ := mime.TypeByExtension(path.Ext(name))
			if typ == "" {
				typ = "application/octet-stream"
			}
			rw.Header().Set("Content-Type", typ)
			http.ServeContent(rw, req, name, startTime, fd.(io.ReadSeeker))
			return
		}
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(rw, req, "index.html", startTime, strings.NewReader(dsbIndexHtml))
}
