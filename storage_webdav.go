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

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/studio-b12/gowebdav"
	"gopkg.in/yaml.v3"

	"github.com/LiterMC/go-openbmclapi/internal/gosrc"
)

type WebDavStorageOption struct {
	MaxConn           int          `yaml:"max-conn"`
	PreGenMeasures    bool         `yaml:"pre-gen-measures"`
	FollowRedirect    bool         `yaml:"follow-redirect"`
	RedirectLinkCache YAMLDuration `yaml:"redirect-link-cache"`

	Alias      string `yaml:"alias,omitempty"`
	WebDavUser `yaml:",inline,omitempty"`

	aliasUser    *WebDavUser
	fullEndPoint string
}

var (
	_ yaml.Marshaler   = (*WebDavStorageOption)(nil)
	_ yaml.Unmarshaler = (*WebDavStorageOption)(nil)
)

func (o *WebDavStorageOption) MarshalYAML() (any, error) {
	type T WebDavStorageOption
	return (*T)(o), nil
}

func (o *WebDavStorageOption) UnmarshalYAML(n *yaml.Node) (err error) {
	// set default values
	o.MaxConn = 24
	o.PreGenMeasures = false
	o.FollowRedirect = false
	o.RedirectLinkCache = 0
	o.Alias = ""

	type T WebDavStorageOption
	if err = n.Decode((*T)(o)); err != nil {
		return
	}
	return
}

func (o *WebDavStorageOption) GetEndPoint() string {
	return o.fullEndPoint
}

func (o *WebDavStorageOption) GetUsername() string {
	if o.Username != "" {
		return o.Username
	}
	if o.Alias != "" {
		// assert o.aliasUser != nil
		return o.aliasUser.Username
	}
	return ""
}

func (o *WebDavStorageOption) GetPassword() string {
	if o.Password != "" {
		return o.Password
	}
	if o.Alias != "" {
		// assert o.aliasUser != nil
		return o.aliasUser.Password
	}
	return ""
}

type WebDavStorage struct {
	opt WebDavStorageOption

	cache Cache
	cli   *gowebdav.Client
	slots *Semaphore
}

var _ Storage = (*WebDavStorage)(nil)

func init() {
	RegisterStorageFactory(StorageWebdav, StorageFactory{
		New:       func() Storage { return new(WebDavStorage) },
		NewConfig: func() any { return new(WebDavStorageOption) },
	})
}

func (s *WebDavStorage) String() string {
	return fmt.Sprintf("<WebDavStorage endpoint=%q user=%s>", s.opt.GetEndPoint(), s.opt.GetUsername())
}

func (s *WebDavStorage) Options() any {
	return &s.opt
}

func (s *WebDavStorage) SetOptions(newOpts any) {
	s.opt = *(newOpts.(*WebDavStorageOption))
}

func webdavIsHTTPError(err error, code int) bool {
	expect := fmt.Sprintf("%v %v", code, http.StatusText(code))
	return strings.Contains(err.Error(), expect)
}

const ClusterCacheCtxKey = "go-openbmclapi.cluster.cache"

func (s *WebDavStorage) Init(ctx context.Context) (err error) {
	if alias := s.opt.Alias; alias != "" {
		user, ok := config.WebdavUsers[alias]
		if !ok {
			logErrorf("Web dav user %q does not exists", alias)
			os.Exit(1)
		}
		s.opt.aliasUser = user
		var end *url.URL
		if end, err = url.Parse(s.opt.aliasUser.EndPoint); err != nil {
			return
		}
		if s.opt.EndPoint != "" {
			var full *url.URL
			if full, err = end.Parse(s.opt.EndPoint); err != nil {
				return
			}
			s.opt.fullEndPoint = full.String()
		} else {
			s.opt.fullEndPoint = s.opt.aliasUser.EndPoint
		}
	} else {
		s.opt.fullEndPoint = s.opt.EndPoint
	}

	if cache, ok := ctx.Value(ClusterCacheCtxKey).(Cache); ok && cache != nil {
		s.cache = NewCacheWithNamespace(cache, fmt.Sprintf("redirect-cache@%s;%s@", s.opt.GetUsername(), s.opt.GetEndPoint()))
	} else {
		s.cache = NoCache
	}

	s.cli = gowebdav.NewClient(s.opt.GetEndPoint(), s.opt.GetUsername(), s.opt.GetPassword())
	s.cli.SetHeader("User-Agent", ClusterUserAgentFull)

	s.slots = NewSemaphore(s.opt.MaxConn)

	if err := s.cli.Mkdir("measure", 0755); err != nil {
		if !webdavIsHTTPError(err, http.StatusConflict) {
			logWarnf("Could not create measure folder for %s: %v", s.String(), err)
		}
	}
	if s.opt.PreGenMeasures {
		logInfo("Creating measure files at %s", s.String())
		for i := 1; i <= 200; i++ {
			if err := s.createMeasureFile(ctx, i); err != nil {
				os.Exit(2)
			}
		}
		logInfo("Measure files created")
	}
	return
}

func (s *WebDavStorage) putFile(path string, r io.ReadSeeker) error {
	size, err := getFileSize(r)
	if err != nil {
		return err
	}
	target, err := url.JoinPath(s.opt.GetEndPoint(), path)
	if err != nil {
		return err
	}
	logDebugf("Putting %q", target)

	s.slots.Acquire()
	defer s.slots.Release()

	cr := &countReader{r, 0}
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPut, target, cr)
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.opt.GetUsername(), s.opt.GetPassword())
	req.ContentLength = size

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return &HTTPStatusError{Code: res.StatusCode}
	}
}

func (s *WebDavStorage) MaxOpen() int {
	if s.opt.MaxConn <= 0 {
		return 0
	}
	if s.opt.MaxConn > 4 {
		return s.opt.MaxConn - 4
	}
	return 1
}

func (s *WebDavStorage) hashToPath(hash string) string {
	return path.Join("download", hash[0:2], hash)
}

func (s *WebDavStorage) Size(hash string) (int64, error) {
	s.slots.Acquire()
	stat, err := s.cli.Stat(s.hashToPath(hash))
	s.slots.Release()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (s *WebDavStorage) Open(hash string) (r io.ReadCloser, err error) {
	s.slots.Acquire()
	if r, err = s.cli.ReadStream(s.hashToPath(hash)); err != nil {
		s.slots.Release()
		return
	}
	r = s.slots.ProxyReader(r)
	return
}

func (s *WebDavStorage) Create(hash string, r io.ReadSeeker) error {
	return s.putFile(s.hashToPath(hash), r)
}

func (s *WebDavStorage) Remove(hash string) error {
	s.slots.Acquire()
	defer s.slots.Release()
	return s.cli.Remove(s.hashToPath(hash))
}

func (s *WebDavStorage) WalkDir(walker func(hash string, size int64) error) error {
	s.slots.Acquire()
	defer s.slots.Release()

	for _, dir := range hex256 {
		files, err := s.cli.ReadDir(path.Join("download", dir))
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.IsDir() {
				if hash := f.Name(); len(hash) >= 2 && hash[:2] == dir {
					if err := walker(hash, f.Size()); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

var noRedirectCli = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func copyHeader(key string, dst, src http.Header) {
	v := src.Get(key)
	if v != "" {
		dst.Set(key, v)
	}
}

func (s *WebDavStorage) ServeDownload(rw http.ResponseWriter, req *http.Request, hash string, size int64) (int64, error) {
	if !s.opt.FollowRedirect && s.opt.RedirectLinkCache > 0 {
		if location, ok := s.cache.Get(hash); ok {
			// fix the size for Ranged request
			rgs, err := gosrc.ParseRange(req.Header.Get("Range"), size)
			if err == nil && len(rgs) > 0 {
				var newSize int64 = 0
				for _, r := range rgs {
					newSize += r.Length
				}
				if newSize < size {
					size = newSize
				}
			}
			rw.Header().Set("Location", location)
			rw.Header().Set("Cache-Control", fmt.Sprintf("public,max-age=%d", (int64)(s.opt.RedirectLinkCache.Dur().Seconds())))
			rw.WriteHeader(http.StatusFound)
			return size, nil
		}
	}

	target, err := url.JoinPath(s.opt.GetEndPoint(), s.hashToPath(hash))
	if err != nil {
		return 0, err
	}
	tgReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	tgReq.SetBasicAuth(s.opt.GetUsername(), s.opt.GetPassword())
	rangeH := req.Header.Get("Range")
	if rangeH != "" {
		tgReq.Header.Set("Range", rangeH)
	}
	copyHeader("If-Modified-Since", tgReq.Header, req.Header)
	copyHeader("If-Unmodified-Since", tgReq.Header, req.Header)
	copyHeader("If-None-Match", tgReq.Header, req.Header)
	copyHeader("If-Match", tgReq.Header, req.Header)
	copyHeader("If-Range", tgReq.Header, req.Header)

	cli := noRedirectCli
	if s.opt.FollowRedirect {
		cli = http.DefaultClient
	}

	if !s.slots.AcquireWithContext(req.Context()) {
		return 0, req.Context().Err()
	}
	defer s.slots.Release()
	resp, err := cli.Do(tgReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	logDebugf("Requested %q: status=%d", target, resp.StatusCode)

	rwh := rw.Header()
	switch resp.StatusCode / 100 {
	case 3:
		// fix the size for Ranged request
		rgs, err := gosrc.ParseRange(rangeH, size)
		if err == nil && len(rgs) > 0 {
			var newSize int64 = 0
			for _, r := range rgs {
				newSize += r.Length
			}
			if newSize < size {
				size = newSize
			}
		}
		location := resp.Header.Get("Location")
		rwh.Set("Location", location)
		copyHeader("ETag", rwh, resp.Header)
		copyHeader("Last-Modified", rwh, resp.Header)
		if s.opt.RedirectLinkCache > 0 {
			rwh.Set("Cache-Control", fmt.Sprintf("public,max-age=%d", (int64)(s.opt.RedirectLinkCache.Dur().Seconds())))
			s.cache.Set(hash, location, CacheOpt{Expiration: s.opt.RedirectLinkCache.Dur()})
		}
		rw.WriteHeader(resp.StatusCode)
		return size, nil
	case 2:
		copyHeader("ETag", rwh, resp.Header)
		copyHeader("Last-Modified", rwh, resp.Header)
		copyHeader("Content-Length", rwh, resp.Header)
		copyHeader("Content-Range", rwh, resp.Header)
		rw.WriteHeader(resp.StatusCode)
		n, _ := io.Copy(rw, resp.Body)
		return n, nil
	default:
		return 0, &HTTPStatusError{Code: resp.StatusCode}
	}
}

func (s *WebDavStorage) ServeMeasure(rw http.ResponseWriter, req *http.Request, size int) error {
	if err := s.createMeasureFile(req.Context(), size); err != nil {
		return err
	}
	target, err := url.JoinPath(s.opt.GetEndPoint(), "measure", strconv.Itoa(size))
	if err != nil {
		return err
	}
	tgReq, err := http.NewRequestWithContext(req.Context(), http.MethodHead, target, nil)
	if err != nil {
		return err
	}
	tgReq.SetBasicAuth(s.opt.GetUsername(), s.opt.GetPassword())
	rangeH := req.Header.Get("Range")
	if rangeH != "" {
		tgReq.Header.Set("Range", rangeH)
	}
	copyHeader("If-Modified-Since", tgReq.Header, req.Header)
	copyHeader("If-Unmodified-Since", tgReq.Header, req.Header)
	copyHeader("If-None-Match", tgReq.Header, req.Header)
	copyHeader("If-Match", tgReq.Header, req.Header)
	copyHeader("If-Range", tgReq.Header, req.Header)

	if !s.slots.AcquireWithContext(req.Context()) {
		return req.Context().Err()
	}
	defer s.slots.Release()
	resp, err := noRedirectCli.Do(tgReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	logDebugf("Requested %q: status=%d", target, resp.StatusCode)

	rwh := rw.Header()
	switch resp.StatusCode / 100 {
	case 3:
		copyHeader("Location", rwh, resp.Header)
		copyHeader("ETag", rwh, resp.Header)
		copyHeader("Last-Modified", rwh, resp.Header)
		rw.WriteHeader(resp.StatusCode)
		return nil
	case 2:
		// Do not read empty file from webdav, it's not helpful
		fallthrough
	default:
		rw.Header().Set("Content-Length", strconv.Itoa(size*mbChunkSize))
		rw.WriteHeader(http.StatusOK)
		if req.Method == http.MethodGet {
			for i := 0; i < size; i++ {
				rw.Write(mbChunk[:])
			}
		}
		return nil
	}
}

func (s *WebDavStorage) createMeasureFile(ctx context.Context, size int) (err error) {
	t := path.Join("measure", strconv.Itoa(size))
	tsz := (int64)(size) * mbChunkSize
	if size == 0 {
		tsz = 2
	}
	if stat, err := s.cli.Stat(t); err == nil {
		sz := stat.Size()
		if sz == tsz {
			return nil
		}
		logDebugf("File [%d] size %d does not match %d", size, sz, tsz)
	} else if e := ctx.Err(); e != nil {
		return e
	} else if !errors.Is(err, os.ErrNotExist) {
		logErrorf("Cannot get stat of %s: %v", t, err)
	}
	logInfof("Creating measure file at %q", t)
	if err = s.putFile(t, io.NewSectionReader(EmptyReader, 0, tsz)); err != nil {
		logErrorf("Cannot create measure file %q: %v", t, err)
		return
	}
	return nil
}
