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

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/studio-b12/gowebdav"
	"gopkg.in/yaml.v3"

	gocache "github.com/LiterMC/go-openbmclapi/cache"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/internal/gosrc"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type WebDavStorageOption struct {
	MaxConn           int                `yaml:"max-conn"`
	MaxUploadRate     int                `yaml:"max-upload-rate"`
	MaxDownloadRate   int                `yaml:"max-download-rate"`
	PreGenMeasures    bool               `yaml:"pre-gen-measures"`
	FollowRedirect    bool               `yaml:"follow-redirect"`
	RedirectLinkCache utils.YAMLDuration `yaml:"redirect-link-cache"`

	Alias      string `yaml:"alias,omitempty"`
	WebDavUser `yaml:",inline,omitempty"`

	AliasUser    *WebDavUser `yaml:"-"`
	FullEndPoint string      `yaml:"-"`
}

type WebDavUser struct {
	EndPoint string `yaml:"endpoint,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
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
	o.MaxConn = 64
	o.MaxUploadRate = 0
	o.MaxDownloadRate = 0
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
	return o.FullEndPoint
}

func (o *WebDavStorageOption) GetUsername() string {
	if o.Username != "" {
		return o.Username
	}
	if o.Alias != "" {
		// assert o.aliasUser != nil
		return o.AliasUser.Username
	}
	return ""
}

func (o *WebDavStorageOption) GetPassword() string {
	if o.Password != "" {
		return o.Password
	}
	if o.Alias != "" {
		// assert o.aliasUser != nil
		return o.AliasUser.Password
	}
	return ""
}

type WebDavStorage struct {
	opt WebDavStorageOption

	cache         gocache.Cache
	cli           *gowebdav.Client
	limitedDialer *limited.LimitedDialer
	httpCli       *http.Client
	noRedCli      *http.Client // no redirect client

	measures  utils.SyncMap[int, struct{}]
	working   atomic.Int32
	checkMux  sync.RWMutex
	lastCheck time.Time
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
	if s.opt.GetEndPoint() == "" {
		return errors.New("Webdav endpoint cannot be empty")
	}

	if cache, ok := ctx.Value(ClusterCacheCtxKey).(gocache.Cache); ok && cache != nil {
		s.cache = gocache.NewCacheWithNamespace(cache, fmt.Sprintf("redirect-cache@%s;%s@", s.opt.GetUsername(), s.opt.GetEndPoint()))
	} else {
		s.cache = gocache.NoCache
	}

	s.limitedDialer = limited.NewLimitedDialer(nil, s.opt.MaxConn, s.opt.MaxDownloadRate*1024, s.opt.MaxUploadRate*1024)
	s.limitedDialer.SetMinReadRate(1024)
	s.limitedDialer.SetMinWriteRate(1024)

	s.httpCli = &http.Client{
		Transport: &http.Transport{
			DialContext: s.limitedDialer.DialContext,
		},
	}
	s.noRedCli = new(http.Client)
	*s.noRedCli = *s.httpCli
	s.noRedCli.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	s.cli = gowebdav.NewClient(s.opt.GetEndPoint(), s.opt.GetUsername(), s.opt.GetPassword())
	s.cli.SetHeader("User-Agent", build.ClusterUserAgentFull)

	if _, err = s.cli.ReadDir(""); err != nil {
		return
	}

	if err := s.cli.Mkdir("measure", 0755); err != nil {
		if !webdavIsHTTPError(err, http.StatusConflict) {
			log.Warnf("Cannot create measure folder for %s: %v", s.String(), err)
		}
	}
	if s.opt.PreGenMeasures {
		log.Info("Creating measure files at %s", s.String())
		for i := 1; i <= 200; i++ {
			if err := s.createMeasureFile(ctx, i); err != nil {
				log.Errorf("Cannot create measure file %d", i)
			}
		}
		log.Info("Measure files created")
	}
	s.working.Store(1)
	return
}

func (s *WebDavStorage) putFile(path string, r io.ReadSeeker) error {
	return s.putFileWithClient(s.httpCli, path, r)
}

func (s *WebDavStorage) putFileWithClient(cli *http.Client, path string, r io.ReadSeeker) error {
	size, err := utils.GetReaderRemainSize(r)
	if err != nil {
		return err
	}
	target, err := url.JoinPath(s.opt.GetEndPoint(), path)
	if err != nil {
		return err
	}
	log.Debugf("Putting %q", target)

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPut, target, io.NopCloser(r))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.opt.GetUsername(), s.opt.GetPassword())
	req.Header.Set("User-Agent", build.ClusterUserAgentFull)
	req.ContentLength = size

	res, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return utils.NewHTTPStatusErrorFromResponse(res)
	}
}

func (s *WebDavStorage) hashToPath(hash string) string {
	return path.Join("download", hash[0:2], hash)
}

func (s *WebDavStorage) Size(hash string) (int64, error) {
	s.limitedDialer.Acquire()
	stat, err := s.cli.Stat(s.hashToPath(hash))
	s.limitedDialer.Release()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (s *WebDavStorage) Open(hash string) (r io.ReadCloser, err error) {
	return s.limitedDialer.DoReader(func() (io.Reader, error) {
		return s.cli.ReadStream(s.hashToPath(hash))
	})
}

func (s *WebDavStorage) Create(hash string, r io.ReadSeeker) error {
	return s.putFile(s.hashToPath(hash), r)
}

func (s *WebDavStorage) Remove(hash string) error {
	return s.cli.Remove(s.hashToPath(hash))
}

func (s *WebDavStorage) WalkDir(walker func(hash string, size int64) error) error {
	done := make(chan struct{}, len(utils.Hex256))
	fileCh := make(chan fs.FileInfo, 0)
	for _, dir := range utils.Hex256 {
		done <- struct{}{}
		go func(dir string) {
			s.limitedDialer.Acquire()
			defer s.limitedDialer.Release()
			defer func() {
				<-done
			}()

			files, err := s.cli.ReadDir(path.Join("download", dir))
			if err != nil {
				return
			}
			for _, f := range files {
				if !f.IsDir() {
					if hash := f.Name(); len(hash) >= 2 && hash[:2] == dir {
						fileCh <- f
					}
				}
			}
		}(dir)
	}
	count := len(utils.Hex256)
	for {
		select {
		case done <- struct{}{}:
			count--
			if count <= 0 {
				return nil
			}
		case f := <-fileCh:
			if err := walker(f.Name(), f.Size()); err != nil {
				return err
			}
		}
	}
}

func copyHeader(key string, dst, src http.Header) {
	v := src.Get(key)
	if v != "" {
		dst.Set(key, v)
	}
}

func (s *WebDavStorage) preServe(ctx context.Context) bool {
	const checkInterval = time.Minute * 3
	now := time.Now()
	if s.working.Load() != 1 {
		if !s.working.CompareAndSwap(0, 2) {
			return false
		}
		s.checkMux.Lock()
		needCheck := now.Sub(s.lastCheck) > checkInterval
		if needCheck {
			s.lastCheck = now
		}
		s.checkMux.Unlock()
		if !needCheck {
			s.working.Store(0)
			return false
		}
		tctx, cancel := context.WithTimeout(ctx, time.Second*5)
		err := s.checkAlive(tctx, 0)
		cancel()
		if err != nil {
			s.working.Store(0)
			return false
		}
		log.Warnf("Re-enabled storage %s", s.String())
		s.working.Store(1)
	} else {
		s.checkMux.RLock()
		lastCheck := s.lastCheck
		s.checkMux.RUnlock()
		if now.Sub(lastCheck) > checkInterval {
			go func() {
				s.checkMux.Lock()
				if s.lastCheck != lastCheck {
					s.checkMux.Unlock()
					return
				}
				s.lastCheck = now
				s.checkMux.Unlock()

				tctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()
				if err := s.checkAlive(tctx, 0); err == nil {
					s.working.Store(1)
				} else {
					log.Errorf("Disabled storage %s: %v", s.String(), err)
					s.working.Store(0)
				}
			}()
		}
	}
	return true
}

func (s *WebDavStorage) ServeDownload(rw http.ResponseWriter, req *http.Request, hash string, size int64) (int64, error) {
	if !s.preServe(req.Context()) {
		return 0, ErrNotWorking
	}
	return s.serveDownload(rw, req, hash, size)
}

func (s *WebDavStorage) serveDownload(rw http.ResponseWriter, req *http.Request, hash string, size int64) (int64, error) {
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
			rw.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", (int64)(s.opt.RedirectLinkCache.Dur().Seconds())))
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
	tgReq.Header.Set("User-Agent", build.ClusterUserAgentFull)
	rangeH := req.Header.Get("Range")
	if rangeH != "" {
		tgReq.Header.Set("Range", rangeH)
	}
	copyHeader("If-Modified-Since", tgReq.Header, req.Header)
	copyHeader("If-Unmodified-Since", tgReq.Header, req.Header)
	copyHeader("If-None-Match", tgReq.Header, req.Header)
	copyHeader("If-Match", tgReq.Header, req.Header)
	copyHeader("If-Range", tgReq.Header, req.Header)

	cli := s.noRedCli
	if s.opt.FollowRedirect {
		cli = s.httpCli
	}

	if !s.limitedDialer.AcquireWithContext(req.Context()) {
		return 0, req.Context().Err()
	}
	defer s.limitedDialer.Release()
	resp, err := cli.Do(tgReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	log.Debugf("Requested %q: status=%d", target, resp.StatusCode)

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
		cacheCtrl := resp.Header.Get("Cache-Control")
		if cacheCtrl != "" {
			rwh.Set("Cache-Control", cacheCtrl)
		} else if s.opt.RedirectLinkCache > 0 {
			rwh.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", (int64)(s.opt.RedirectLinkCache.Dur().Seconds())))
			s.cache.Set(hash, location, gocache.CacheOpt{Expiration: s.opt.RedirectLinkCache.Dur()})
		}
		rw.WriteHeader(resp.StatusCode)
		return size, nil
	case 2:
		copyHeader("ETag", rwh, resp.Header)
		copyHeader("Last-Modified", rwh, resp.Header)
		copyHeader("Content-Length", rwh, resp.Header)
		if name := req.URL.Query().Get("name"); name != "" {
			rwh.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		}
		copyHeader("Content-Range", rwh, resp.Header)
		rw.WriteHeader(resp.StatusCode)
		n, _ := io.Copy(rw, resp.Body)
		return n, nil
	default:
		return 0, utils.NewHTTPStatusErrorFromResponse(resp)
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
	tgReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, target, nil)
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

	resp, err := s.noRedCli.Do(tgReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	log.Debugf("Requested %q: status=%d", target, resp.StatusCode)

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
		resp.Body.Close()
		rwh.Set("Content-Length", strconv.Itoa(size*utils.MbChunkSize))
		rw.WriteHeader(http.StatusOK)
		if req.Method == http.MethodGet {
			for i := 0; i < size; i++ {
				rw.Write(utils.MbChunk[:])
			}
		}
		return nil
	}
}

func (s *WebDavStorage) createMeasureFile(ctx context.Context, size int) error {
	if s.measures.Has(size) {
		// TODO: is this safe?
		return nil
	}
	t := path.Join("measure", strconv.Itoa(size))
	tsz := (int64)(size) * utils.MbChunkSize
	if size == 0 {
		tsz = 2
	}
	if stat, err := s.cli.Stat(t); err == nil {
		sz := stat.Size()
		if sz == tsz {
			return nil
		}
		log.Debugf("File [%d] size %d does not match %d", size, sz, tsz)
	} else if e := ctx.Err(); e != nil {
		return e
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Errorf("Cannot get stat of %s: %v", t, err)
	}
	log.Infof("Creating measure file at %q", t)
	if err := s.putFile(t, io.NewSectionReader(utils.EmptyReader, 0, tsz)); err != nil {
		log.Errorf("Cannot create measure file %q: %v", t, err)
		return err
	}
	s.measures.Set(size, struct{}{})
	return nil
}

func (s *WebDavStorage) checkAlive(ctx context.Context, size int) (err error) {
	var targetSize int64
	if size == 0 {
		targetSize = 2
	} else {
		targetSize = (int64)(size) * 1024 * 1024
	}
	log.Infof("Checking %s for %d bytes ...", s.String(), targetSize)

	if err = s.createMeasureFile(ctx, size); err != nil {
		return
	}
	target, err := url.JoinPath(s.opt.GetEndPoint(), "measure", strconv.Itoa(size))
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return
	}
	req.SetBasicAuth(s.opt.GetUsername(), s.opt.GetPassword())
	req.Header.Set("User-Agent", build.ClusterUserAgentFull)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return utils.NewHTTPStatusErrorFromResponse(resp)
	}
	if resp.ContentLength >= 0 && resp.ContentLength != targetSize {
		return fmt.Errorf("Content-Length not match, got %d, expect %d", resp.ContentLength, targetSize)
	}
	start := time.Now()
	n, err := io.Copy(io.Discard, resp.Body)
	used := time.Since(start)
	if err != nil {
		return fmt.Errorf("WebDavStorage check failed %q: %w", target, err)
	}
	if n != targetSize {
		return fmt.Errorf("Content-Length not match, got %d, expect %d", resp.ContentLength, targetSize)
	}
	rate := (float64)(n) / used.Seconds()
	log.Infof("Check finished for %q, used %v, %s/s", target, used, utils.BytesToUnit(rate))
	return
}

func (s *WebDavStorage) CheckUpload(ctx context.Context) (err error) {
	const fileName = ".check"
	log.Infof("Checking upload at %s ...", s.String())

	data := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if err = s.putFileWithClient(http.DefaultClient, fileName, strings.NewReader(data)); err != nil {
		return err
	}

	target, err := url.JoinPath(s.opt.GetEndPoint(), fileName)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return
	}
	req.SetBasicAuth(s.opt.GetUsername(), s.opt.GetPassword())
	req.Header.Set("User-Agent", build.ClusterUserAgentFull)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return utils.NewHTTPStatusErrorFromResponse(resp)
	}
	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if sres := (string)(res); sres != data {
		return fmt.Errorf("Content not match, expected %q, got %q", data, sres)
	}

	if err = s.cli.Remove(fileName); err != nil {
		return
	}
	return
}
