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
	"io"
	"os"
)

func cmdUploadWebdav(args []string) {
	config := readConfig()

	var localOpt *LocalStorageOption
	webdavOpts := make([]*WebDavStorageOption, 0, 4)
	for _, s := range config.Storages {
		switch s := s.Data.(type) {
		case *LocalStorageOption:
			if localOpt == nil {
				localOpt = s
			}
		case *WebDavStorageOption:
			webdavOpts = append(webdavOpts, s)
		}
	}

	if localOpt == nil {
		logError("At least one local storage is required")
		os.Exit(1)
	}
	if len(webdavOpts) == 0 {
		logError("At least one webdav storage is required")
		os.Exit(1)
	}

	ctx := context.Background()

	var local LocalStorage
	local.SetOptions(localOpt)
	if err := local.Init(ctx); err != nil {
		logErrorf("Cannot initialize %s: %v", local.String(), err)
		os.Exit(1)
	}
	logInfof("From: %s", local.String())

	webdavs := make([]*WebDavStorage, len(webdavOpts))
	for i, opt := range webdavOpts {
		s := new(WebDavStorage)
		s.SetOptions(opt)
		if err := s.Init(ctx); err != nil {
			logErrorf("Cannot initialize %s: %v", s.String(), err)
			os.Exit(1)
		}
		logInfof("To: %s", s.String())
		webdavs[i] = s
	}

	for _, s := range webdavs {
		local.WalkDir(func(hash string) error {
			size, err := local.Size(hash)
			if err != nil {
				logErrorf("Cannot get stat of %s: %v", hash, err)
				return nil
			}
			if sz, err := s.Size(hash); err == nil && sz == size {
				return nil
			}

			r, err := local.Open(hash)
			if err != nil {
				logErrorf("Cannot open %s: %v", hash, err)
				return nil
			}
			defer r.Close()
			w, err := s.Create(hash)
			if err != nil {
				logErrorf("Cannot create %s at %s: %v", hash, s.String(), err)
				return err
			}

			var buf [1024 * 1024]byte
			_, err = io.CopyBuffer(w, r, buf[:])
			if e := w.Close(); e != nil {
				if err == nil {
					err = e
				} else {
					err = errors.Join(err, e)
				}
			}
			if err != nil {
				s.Remove(hash)
				logErrorf("Cannot copy %s to %s: %v", hash, s.String(), err)
				return nil
			}
			logInfof("File %s copied to %s", hash, s.String())
			return nil
		})
	}
}
