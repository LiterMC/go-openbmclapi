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
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"

	"github.com/gregjones/httpcache"

	gocache "github.com/LiterMC/go-openbmclapi/cache"
	"github.com/LiterMC/go-openbmclapi/internal/build"
)

type HTTPClient struct {
	cli, cachedCli *http.Client
}

func NewHTTPClient(dialer *net.Dialer, cache gocache.Cache) *HTTPClient {
	transport := http.DefaultTransport
	if dialer != nil {
		transport = &http.Transport{
			DialContext: dialer.DialContext,
		}
	}
	cachedTransport := transport
	if cache != gocache.NoCache {
		cachedTransport = &httpcache.Transport{
			Transport: transport,
			Cache:     gocache.WrapToHTTPCache(gocache.NewCacheWithNamespace(cache, "http@")),
		}
	}
	return &HTTPClient{
		cli: &http.Client{
			Transport:     transport,
			CheckRedirect: redirectChecker,
		},
		cachedCli: &http.Client{
			Transport:     cachedTransport,
			CheckRedirect: redirectChecker,
		},
	}
}

func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.cli.Do(req)
}

func (c *HTTPClient) DoUseCache(req *http.Request) (*http.Response, error) {
	return c.cachedCli.Do(req)
}

func redirectChecker(req *http.Request, via []*http.Request) error {
	req.Header.Del("Referer")
	if len(via) > 10 {
		return errors.New("More than 10 redirects detected")
	}
	return nil
}

func (cr *Cluster) makeReq(ctx context.Context, method string, relpath string, query url.Values) (req *http.Request, err error) {
	return cr.makeReqWithBody(ctx, method, relpath, query, nil)
}

func (cr *Cluster) makeReqWithBody(
	ctx context.Context,
	method string, relpath string,
	query url.Values, body io.Reader,
) (req *http.Request, err error) {
	var u *url.URL
	if u, err = url.Parse(cr.opts.Server); err != nil {
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
