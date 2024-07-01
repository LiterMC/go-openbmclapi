/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2024 Kevin Z <zyxkad@gmail.com>
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

package cluster

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/LiterMC/go-openbmclapi/internal/build"
)

func (cr *Cluster) makeReq(ctx context.Context, method string, relpath string, query url.Values) (req *http.Request, err error) {
	return cr.makeReqWithBody(ctx, method, relpath, query, nil)
}

func (cr *Cluster) makeReqWithBody(
	ctx context.Context,
	method string, relpath string,
	query url.Values, body io.Reader,
) (req *http.Request, err error) {
	var u *url.URL
	if u, err = url.Parse(cr.opts.Prefix); err != nil {
		return
	}
	u.Path = path.Join(u.Path, relpath)
	if query != nil {
		u.RawQuery = query.Encode()
	}
	target := u.String()

	req, err = http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", build.ClusterUserAgent)
	return
}

func (cr *Cluster) makeReqWithAuth(ctx context.Context, method string, relpath string, query url.Values) (req *http.Request, err error) {
	req, err = cr.makeReq(ctx, method, relpath, query)
	if err != nil {
		return
	}
	token, err := cr.GetAuthToken(ctx)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return
}
