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
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type LocalStorageOption struct {
	CachePath  string     `yaml:"cache-path"`
	Compressor Compressor `yaml:"compressor"`
}

type LocalStorage struct {
	opt LocalStorageOption
}

var _ Storage = (*LocalStorage)(nil)

func init() {
	RegisterStorageFactory(StorageLocal, StorageFactory{
		New:       func() Storage { return new(LocalStorage) },
		NewConfig: func() any { return new(LocalStorageOption) },
	})
}

func (s *LocalStorage) String() string {
	return fmt.Sprintf("<LocalStorage cache=%q>", s.opt.CachePath)
}

func (s *LocalStorage) Options() any {
	return &s.opt
}

func (s *LocalStorage) SetOptions(newOpts any) {
	s.opt = *(newOpts.(*LocalStorageOption))
}

func (s *LocalStorage) Init(context.Context) (err error) {
	if err = initCache(s.opt.CachePath); err != nil {
		return
	}
	return
}

func initCache(base string) (err error) {
	if err = os.MkdirAll(base, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return
	}
	var b [1]byte
	for i := 0; i < 0x100; i++ {
		b[0] = (byte)(i)
		if err = os.Mkdir(filepath.Join(base, hex.EncodeToString(b[:])), 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return
		}
	}
	return nil
}

func (s *LocalStorage) hashToPath(hash string) string {
	return filepath.Join(s.opt.CachePath, hash[0:2], hash)
}

func (s *LocalStorage) Size(hash string) (int64, error) {
	stat, err := os.Stat(s.hashToPath(hash))
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (s *LocalStorage) OpenFd(hash string) (*os.File, error) {
	return os.Open(s.hashToPath(hash))
}

func (s *LocalStorage) Open(hash string) (io.ReadCloser, error) {
	return s.OpenFd(hash)
}

func (s *LocalStorage) Create(hash string, r io.ReadSeeker) error {
	fd, err := os.Create(s.hashToPath(hash))
	if err != nil {
		return err
	}
	var buf [1024 * 512]byte
	_, err = io.CopyBuffer(fd, r, buf[:])
	if e := fd.Close(); e != nil && err == nil {
		err = e
	}
	return err
}

func (s *LocalStorage) Remove(hash string) error {
	return os.Remove(s.hashToPath(hash))
}

func (s *LocalStorage) WalkDir(walker func(hash string, size int64) error) error {
	return utils.WalkCacheDir(s.opt.CachePath, walker)
}

func (s *LocalStorage) ServeDownload(rw http.ResponseWriter, req *http.Request, hash string, size int64) (int64, error) {
	acceptEncoding := utils.SplitCSV(req.Header.Get("Accept-Encoding"))
	name := req.URL.Query().Get("name")

	hasGzip := false
	isGzip := false
	path := s.hashToPath(hash)
	if s.opt.Compressor == GzipCompressor {
		if _, err := os.Stat(path + ".gz"); err == nil {
			hasGzip = true
		}
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if !hasGzip {
			return 0, err
		}
		if hasGzip {
			isGzip = true
			path += ".gz"
		}
	}

	if !isGzip && rw.Header().Get("Range") != "" {
		fd, err := os.Open(path)
		if err != nil {
			return 0, err
		}
		defer fd.Close()

		counter := &utils.CountReader{
			ReadSeeker: fd,
		}
		rw.Header().Set("ETag", `"`+hash+`"`)
		rw.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // cache for a year
		rw.Header().Set("Content-Type", "application/octet-stream")
		if name != "" {
			rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		}
		http.ServeContent(rw, req, name, time.Time{}, counter)
		return counter.N, nil
	}

	var r io.Reader
	if hasGzip && acceptEncoding["gzip"] != 0 {
		if !isGzip {
			isGzip = true
			path += ".gz"
		}
		fd, err := os.Open(path)
		if err != nil {
			return 0, err
		}
		defer fd.Close()
		r = fd
		size, _ = utils.GetReaderRemainSize(fd)
		rw.Header().Set("Content-Encoding", "gzip")
	} else {
		fd, err := os.Open(path)
		if err != nil {
			return 0, err
		}
		defer fd.Close()
		r = fd
		if isGzip {
			size = 0
			if r, err = gzip.NewReader(r); err != nil {
				log.Errorf("Could not decompress %q: %v", path, err)
				return 0, err
			}
			isGzip = false
		}
	}
	rw.Header().Set("ETag", `"`+hash+`"`)
	rw.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // cache for a year
	rw.Header().Set("Content-Type", "application/octet-stream")
	if name != "" {
		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	}
	if size > 0 {
		rw.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	rw.WriteHeader(http.StatusOK)
	if req.Method == http.MethodGet {
		buf, free := utils.AllocBuf()
		defer free()
		n, _ := io.CopyBuffer(rw, r, buf)
		return n, nil
	}
	return 0, nil
}

func (s *LocalStorage) ServeMeasure(rw http.ResponseWriter, req *http.Request, size int) error {
	rw.Header().Set("Content-Length", strconv.Itoa(size*utils.MbChunkSize))
	rw.WriteHeader(http.StatusOK)
	if req.Method == http.MethodGet {
		for i := 0; i < size; i++ {
			rw.Write(utils.MbChunk[:])
		}
	}
	return nil
}
