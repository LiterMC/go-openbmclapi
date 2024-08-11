// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gosrc

import (
	"net/http"
	"net/url"
	"strings"
)

func RequestStripPrefix(r *http.Request, prefix string) *http.Request {
	p, ok := strings.CutPrefix(r.URL.Path, prefix)
	rp, ok2 := strings.CutPrefix(r.URL.RawPath, prefix)
	if ok && (ok2 || r.URL.RawPath == "") {
		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path = p
		r2.URL.RawPath = rp
		return r2
	}
	return nil
}
