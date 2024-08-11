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

package sub_commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/LiterMC/go-openbmclapi/config"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/utils"
)

func CmdUploadWebdav(args []string) {
	cfg := readConfig()

	var (
		localOpt   storage.StorageOption
		webdavOpts = make([]storage.StorageOption, 0, 4)
	)
	for _, s := range cfg.Storages {
		switch s.Data.(type) {
		case *storage.LocalStorageOption:
			if localOpt.Data == nil {
				localOpt = s
			}
		case *storage.WebDavStorageOption:
			webdavOpts = append(webdavOpts, s)
		}
	}

	if localOpt.Data == nil {
		log.Error("At least one local storage is required")
		os.Exit(1)
	}
	if len(webdavOpts) == 0 {
		log.Error("At least one webdav storage is required")
		os.Exit(1)
	}

	ctx := context.Background()

	local := storage.NewLocalStorage(localOpt)
	if err := local.Init(ctx); err != nil {
		log.Errorf("Cannot initialize %s: %v", local.String(), err)
		os.Exit(1)
	}
	log.Infof("From: %s", local.String())

	webdavs := make([]*storage.WebDavStorage, len(webdavOpts))
	maxProc := 0
	for i, opt := range webdavOpts {
		if maxConn := opt.Data.(*storage.WebDavStorageOption).MaxConn; maxConn > maxProc {
			maxProc = maxConn
		}
		s := storage.NewWebDavStorage(opt)
		if err := s.Init(ctx); err != nil {
			log.Errorf("Cannot initialize %s: %v", s.String(), err)
			os.Exit(1)
		}
		log.Infof("To: %s", s.String())
		webdavs[i] = s
	}

	if maxProc < 1 {
		maxProc = runtime.GOMAXPROCS(0) * 4
		if maxProc < 1 {
			maxProc = 1
		}
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

	var barUnit decor.SizeB1024
	var wg sync.WaitGroup
	pg := mpb.New(mpb.WithWaitGroup(&wg), mpb.WithRefreshRate(time.Second), mpb.WithAutoRefresh())
	log.SetLogOutput(pg)

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
	webdavBar.SetTotal((int64)(len(localFiles)*2*len(webdavs)), false)
	for i, s := range webdavs {
		start := time.Now()
		fileSet := make(map[string]int64)
		err := s.WalkDir(func(hash string, size int64) error {
			fileSet[hash] = size
			now := time.Now()
			webdavBar.EwmaIncrement(now.Sub(start))
			start = now
			return nil
		})
		if err != nil {
			log.Errorf("Cannot walk %s: %v", s, err)
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
			now := time.Now()
			webdavBar.EwmaIncrement(now.Sub(start))
			start = now
		}
		totalFiles += (int64)(len(files))
		webdavFiles[i] = files
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

	log.Infof("Max Proc: %d", maxProc)

	for i, files := range webdavFiles {
		s := webdavs[i]
		log.Infof("Storage %s need sync %d files", s, len(files))
		for _, info := range files {
			hash, size := info.Hash, info.Size
			slot := <-slots

			fd, err := local.OpenFd(hash)
			if err != nil {
				putSlot(slot)
				log.Errorf("Cannot open %s: %v", hash, err)
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
			go func(slot int, bar *mpb.Bar, s *storage.WebDavStorage, hash string, size int64) {
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

				log.Debugf("Uploading %s/%s", s.String(), hash)
				err := s.Create(hash, utils.ProxyPBReadSeeker(fd, bar, totalBar, lastInc))
				uploadedFiles.Add(1)
				if err != nil {
					log.Errorf("Cannot create %s at %s: %v", hash, s.String(), err)
					return
				}
				log.Infof("File %s uploaded to %s", hash, s.String())
			}(slot, bar, s, hash, size)
		}
	}

	pg.Wait()
	log.SetLogOutput(nil)
}

func readConfig() (cfg *config.Config) {
	const configPath = "config.yaml"

	cfg = config.NewDefaultConfig()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.TrErrorf("error.config.read.failed", err)
			os.Exit(1)
		}
		log.TrErrorf("error.config.not.exists")
		os.Exit(1)
	}
	if err = cfg.UnmarshalText(data); err != nil {
		log.TrErrorf("error.config.parse.failed", err)
		os.Exit(1)
	}
	return
}
