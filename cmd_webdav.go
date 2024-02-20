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
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

func cmdUploadWebdav(args []string) {
	config = readConfig()

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

	var barUnit decor.SizeB1024
	maxProc := runtime.GOMAXPROCS(0) * 4
	if maxProc < 1 {
		maxProc = 1
	}
	slots := make(chan int, maxProc)
	for i := 0; i < maxProc; i++ {
		slots <- i
	}
	putSlot := func(slot int) {
		slots <- slot
	}

	type fileInfo struct {
		Hash string
		Size int64
	}

	var (
		totalSize, totalFiles int64
		uploadedFiles         atomic.Int64

		localFiles  = make(map[string]int64)
		webdavFiles = make([][]fileInfo, len(webdavs))
	)
	local.WalkDir(func(hash string, size int64) error {
		localFiles[hash] = size
		return nil
	})

	var wg sync.WaitGroup
	pg := mpb.New(mpb.WithWaitGroup(&wg), mpb.WithAutoRefresh())
	setLogOutput(pg)

	webdavBar := pg.AddBar(0,
		mpb.BarRemoveOnComplete(),
		mpb.BarPriority(maxProc),
		mpb.PrependDecorators(
			decor.Name("Counting webdav objects"),
		),
		mpb.AppendDecorators(
			decor.CountersNoUnit("(%d / %d) "),
			decor.EwmaETA(decor.ET_STYLE_GO, 30),
		),
	)
	webdavBar.SetTotal((int64)(len(localFiles)*len(webdavs)), false)
	for i, s := range webdavs {
		start := time.Now()
		fileSet := make(map[string]int64)
		err := s.WalkDir(func(hash string, size int64) error {
			fileSet[hash] = size
			return nil
		})
		if err != nil {
			logErrorf("Cannot walk %s: %v", s, err)
			os.Exit(2)
		}
		files := make([]fileInfo, 0, 100)
		for hash, size := range localFiles {
			if fileSet[hash] != size {
				files = append(files, fileInfo{
					Hash: hash,
					Size: size,
				})
				totalSize += size
			}
		}
		totalFiles += (int64)(len(files))
		webdavFiles[i] = files
		webdavBar.EwmaIncrement(time.Since(start))
	}
	webdavBar.SetTotal(-1, true)
	webdavBar.Wait()

	lastInc := new(atomic.Int64)
	lastInc.Store(time.Now().UnixNano())
	totalBar := pg.AddBar(totalSize,
		mpb.BarRemoveOnComplete(),
		mpb.BarPriority(maxProc),
		mpb.PrependDecorators(
			decor.Name("Total Uploads: "),
			decor.NewPercentage("%.2f"),
		),
		mpb.AppendDecorators(
			decor.Any(func(decor.Statistics) string {
				return fmt.Sprintf("(%d / %d) ", uploadedFiles.Load(), totalFiles)
			}),
			decor.Counters(barUnit, "(%.1f / %.1f) "),
			decor.EwmaSpeed(barUnit, "%.1f ", 30),
			decor.OnComplete(
				decor.EwmaETA(decor.ET_STYLE_GO, 30), "done",
			),
		),
	)

	logDebugf("Max Proc: %d", maxProc)

	for i, files := range webdavFiles {
		s := webdavs[i]
		logInfof("Storage %s need sync %d files", s, len(files))
		for _, info := range files {
			hash, size := info.Hash, info.Size
			slot := <-slots

			fd, err := local.OpenFd(hash)
			if err != nil {
				putSlot(slot)
				logErrorf("Cannot open %s: %v", hash, err)
				continue
			}

			bar := pg.AddBar(0,
				mpb.BarPriority(slot),
				mpb.PrependDecorators(
					decor.Name(fmt.Sprintf("> Uploading %s/%s", s.String(), hash), decor.WCSyncSpaceR),
				),
				mpb.AppendDecorators(
					decor.NewPercentage("%d", decor.WCSyncSpace),
					decor.Counters(barUnit, "(%.1f / %.1f)", decor.WCSyncSpace),
					decor.EwmaSpeed(barUnit, "%.1f", 10, decor.WCSyncSpace),
					decor.OnComplete(
						decor.EwmaETA(decor.ET_STYLE_GO, 10, decor.WCSyncSpace), "done",
					),
				),
			)
			wg.Add(1)
			go func(slot int, bar *mpb.Bar, s Storage, hash string, size int64) {
				defer putSlot(slot)
				defer wg.Done()
				defer func() {
					bar.Abort(true)
					need := size - bar.Current()
					if need > 0 {
						totalBar.IncrInt64(need)
					}
				}()
				defer fd.Close()

				bar.SetTotal(size, false)

				logDebugf("Uploading %s/%s", s.String(), hash)
				err := s.Create(hash, ProxyReadSeeker(fd, bar, totalBar, lastInc))
				uploadedFiles.Add(1)
				if err != nil {
					logErrorf("Cannot create %s at %s: %v", hash, s.String(), err)
					return
				}
				logInfof("File %s uploaded to %s", hash, s.String())
			}(slot, bar, s, hash, size)
		}
	}

	pg.Wait()
	setLogOutput(nil)
}
