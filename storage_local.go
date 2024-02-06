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
	"errors"
	"io"
	"net/http"
	"path/filepath"
)

type LocalStorageOption struct {
	CachePath  string     `yaml:"cache-path"`
	Compressor Compressor `yaml:"compressor"`
}

func (opt *LocalStorageOption) TmpPath() string {
	return filepath.Join(opt.CachePath, ".tmp")
}

type LocalStorage struct {
	opt LocalStorageOption
}

var _ Storage = (*LocalStorage)(nil)

func (s *LocalStorage) String() string {
	return fmt.Sprintf("<LocalStorage cache=%q>", s.CachePath)
}

func (s *LocalStorage) Options() any {
	return &s.opt
}

func (s *LocalStorage) SetOptions(newOpts any) {
	s.opt = *(newOpts.(*LocalStorageOption))
}

func (s *LocalStorage) Init() (err error) {
	tmpDir := s.opt.TmpPath()
	os.RemoveAll(tmpDir)
	// should be 0755 here because Windows permission issue
	if err = os.MkdirAll(tmpDir, 0755); err != nil {
		return
	}
	if err = initCache(s.opt.CachePath); err != nil {
		return
	}
}

func (s *LocalStorage) hashToPath(hash string) string {
	return filepath.Join(s.opt.CachePath, hash[0:2], hash)
}

func (s *LocalStorage) Size(hash string) (int64, error) {
	stat, err := os.Open(s.hashToPath(hash))
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (s *LocalStorage) Open(hash string) (io.ReadCloser, error) {
	return os.Open(s.hashToPath(hash))
}

func (s *LocalStorage) Create(hash string) (io.WriteCloser, error) {
	return os.Create(s.hashToPath(hash))
}

func (s *LocalStorage) Remove(hash string) error {
	return os.Remove(s.hashToPath(hash))
}

func (s *LocalStorage) WalkDir(walker func(hash string) error) error {
	var b [1]byte
	for i := 0; i < 0x100; i++ {
		b[0] = (byte)(i)
		dir := hex.EncodeToString(b[:])
		files, err := os.ReadDir(filepath.Join(s.opt.CachePath, dir))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return
		}
		for _, f := range files {
			if !f.IsDir() {
				if name := f.Name(); len(name) >= 2 && name[:2] == dir {
					if err = walker(f.Name()); err != nil {
						return
					}
				}
			}
		}
	}
	return nil
}

func (s *LocalStorage) ServeDownload(rw http.ResponseWriter, req *http.Request, hash string) error {
	acceptEncoding := splitCSV(req.Header.Get("Accept-Encoding"))
	name := req.URL.Query().Get("name")

	hasGzip := false
	isGzip := false
	path := s.hashToPath(hash)
	if config.UseGzip {
		if _, err := os.Stat(path + ".gz"); err == nil {
			hasGzip = true
		}
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if !hasGzip {
			if hasGzip, err = cr.DownloadFile(req.Context(), hash); err != nil {
				return err
			}
		}
		if hasGzip {
			isGzip = true
			path += ".gz"
		}
	}

	if !isGzip && rw.Header().Get("Range") != "" {
		fd, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fd.Close()

		rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		rw.Header().Set("X-Bmclapi-Hash", hash)
		http.ServeContent(rw, req, name, time.Time{}, fd)
		return nil
	}

	var r io.Reader
	if hasGzip && acceptEncoding["gzip"] != 0 {
		if !isGzip {
			isGzip = true
			path += ".gz"
		}
		fd, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fd.Close()
		r = fd
		rw.Header().Set("Content-Encoding", "gzip")
	} else {
		fd, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fd.Close()
		r = fd
		if isGzip {
			if r, err = gzip.NewReader(r); err != nil {
				logErrorf("Could not decompress %q: %v", path, err)
				return err
			}
			isGzip = false
		}
	}
	rw.Header().Set("Cache-Control", "max-age=2592000") // 30 days
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	rw.Header().Set("X-Bmclapi-Hash", hash)
	if !isGzip {
		if size, ok := cr.FileSet()[hash]; ok {
			rw.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		}
	} else if size, err := getFileSize(r); err == nil {
		rw.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	rw.WriteHeader(http.StatusOK)
	if req.Method != http.MethodHead {
		var buf []byte
		{
			buf0 := bufPool.Get().(*[]byte)
			defer bufPool.Put(buf0)
			buf = *buf0
		}
		io.CopyBuffer(rw, r, buf)
	}
	return nil
}

func (s *LocalStorage) ServeMeasure(rw http.ResponseWriter, req *http.Request, size int) error {
	//
}
