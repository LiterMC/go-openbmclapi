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
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hamba/avro/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/update"
	"github.com/LiterMC/go-openbmclapi/utils"
)

func (cr *Cluster) CloneFileset() map[string]int64 {
	cr.filesetMux.RLock()
	defer cr.filesetMux.RUnlock()
	fileset := make(map[string]int64, len(cr.fileset))
	for k, v := range cr.fileset {
		fileset[k] = v
	}
	return fileset
}

func (cr *Cluster) CachedFileSize(hash string) (size int64, ok bool) {
	cr.filesetMux.RLock()
	defer cr.filesetMux.RUnlock()
	if size, ok = cr.fileset[hash]; !ok {
		return
	}
	if size < 0 {
		size = -size
	}
	return
}

type syncStats struct {
	slots  *limited.BufSlots
	noOpen bool

	totalSize          int64
	okCount, failCount atomic.Int32
	totalFiles         int

	pg       *mpb.Progress
	totalBar *mpb.Bar
	lastInc  atomic.Int64
}

func (cr *Cluster) SyncFiles(ctx context.Context, files []FileInfo, heavyCheck bool) bool {
	log.Infof(Tr("info.sync.prepare"), len(files))
	if !cr.issync.CompareAndSwap(false, true) {
		log.Warn("Another sync task is running!")
		return false
	}
	defer cr.issync.Store(false)

	sort.Slice(files, func(i, j int) bool { return files[i].Hash < files[j].Hash })
	if cr.syncFiles(ctx, files, heavyCheck) != nil {
		return false
	}

	cr.filesetMux.Lock()
	for _, f := range files {
		cr.fileset[f.Hash] = f.Size
	}
	cr.filesetMux.Unlock()

	return true
}

type fileInfoWithTargets struct {
	FileInfo
	tgMux   sync.Mutex
	targets []storage.Storage
}

func (cr *Cluster) checkFileFor(
	ctx context.Context,
	sto storage.Storage, files []FileInfo,
	heavy bool,
	missing *utils.SyncMap[string, *fileInfoWithTargets],
	pg *mpb.Progress,
) (err error) {
	var missingCount atomic.Int32
	addMissing := func(f FileInfo) {
		missingCount.Add(1)
		if info, has := missing.GetOrSet(f.Hash, func() *fileInfoWithTargets {
			return &fileInfoWithTargets{
				FileInfo: f,
				targets:  []storage.Storage{sto},
			}
		}); has {
			info.tgMux.Lock()
			info.targets = append(info.targets, sto)
			info.tgMux.Unlock()
		}
	}

	log.Infof(Tr("info.check.start"), sto.String(), heavy)

	var (
		checkingHashMux  sync.Mutex
		checkingHash     string
		lastCheckingHash string
		slots            *limited.BufSlots
	)

	if heavy {
		slots = limited.NewBufSlots(runtime.GOMAXPROCS(0) * 2)
	}

	bar := pg.AddBar(0,
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name(Tr("hint.check.checking")),
			decor.Name(sto.String()),
			decor.OnCondition(
				decor.Any(func(decor.Statistics) string {
					c, l := slots.Cap(), slots.Len()
					return fmt.Sprintf(" (%d / %d)", c-l, c)
				}),
				heavy,
			),
		),
		mpb.AppendDecorators(
			decor.CountersNoUnit("%d / %d", decor.WCSyncSpaceR),
			decor.NewPercentage("%d", decor.WCSyncSpaceR),
			decor.EwmaETA(decor.ET_STYLE_GO, 60),
		),
		mpb.BarExtender((mpb.BarFillerFunc)(func(w io.Writer, _ decor.Statistics) (err error) {
			if checkingHashMux.TryLock() {
				lastCheckingHash = checkingHash
				checkingHashMux.Unlock()
			}
			if lastCheckingHash != "" {
				_, err = fmt.Fprintln(w, "\t", lastCheckingHash)
			}
			return
		}), false),
	)
	defer bar.Wait()
	defer bar.Abort(true)

	bar.SetTotal(0x100, false)

	sizeMap := make(map[string]int64, len(files))
	{
		start := time.Now()
		var checkedMp [256]bool
		if err = sto.WalkDir(func(hash string, size int64) error {
			if n := utils.HexTo256(hash); !checkedMp[n] {
				checkedMp[n] = true
				now := time.Now()
				bar.EwmaIncrement(now.Sub(start))
				start = now
			}
			sizeMap[hash] = size
			return nil
		}); err != nil {
			return
		}
	}

	bar.SetCurrent(0)
	bar.SetTotal((int64)(len(files)), false)
	for _, f := range files {
		if err = ctx.Err(); err != nil {
			return
		}
		start := time.Now()
		hash := f.Hash
		if checkingHashMux.TryLock() {
			checkingHash = hash
			checkingHashMux.Unlock()
		}
		name := sto.String() + "/" + hash
		if f.Size == 0 {
			log.Debugf("Skipped empty file %s", name)
		} else if size, ok := sizeMap[hash]; ok {
			if size != f.Size {
				log.Warnf(Tr("warn.check.modified.size"), name, size, f.Size)
				addMissing(f)
			} else if heavy {
				hashMethod, err := getHashMethod(len(hash))
				if err != nil {
					log.Errorf(Tr("error.check.unknown.hash.method"), hash)
				} else {
					_, buf, free := slots.Alloc(ctx)
					if buf == nil {
						return ctx.Err()
					}
					go func(f FileInfo, buf []byte, free func()) {
						defer log.RecoverPanic(nil)
						defer free()
						miss := true
						r, err := sto.Open(hash)
						if err != nil {
							log.Errorf(Tr("error.check.open.failed"), name, err)
						} else {
							hw := hashMethod.New()
							_, err = io.CopyBuffer(hw, r, buf[:])
							r.Close()
							if err != nil {
								log.Errorf(Tr("error.check.hash.failed"), name, err)
							} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != hash {
								log.Warnf(Tr("warn.check.modified.hash"), name, hs, hash)
							} else {
								miss = false
							}
						}
						free()
						if miss {
							addMissing(f)
						}
						bar.EwmaIncrement(time.Since(start))
					}(f, buf, free)
					continue
				}
			}
		} else {
			// log.Debugf("Could not found file %q", name)
			addMissing(f)
		}
		bar.EwmaIncrement(time.Since(start))
	}

	checkingHashMux.Lock()
	checkingHash = ""
	checkingHashMux.Unlock()

	bar.SetTotal(-1, true)
	log.Infof(Tr("info.check.done"), sto.String(), missingCount.Load())
	return
}

func (cr *Cluster) CheckFiles(
	ctx context.Context,
	files []FileInfo,
	heavyCheck bool,
	pg *mpb.Progress,
) (map[string]*fileInfoWithTargets, error) {
	missingMap := utils.NewSyncMap[string, *fileInfoWithTargets]()
	done := make(chan bool, 0)

	for _, s := range cr.storages {
		go func(s storage.Storage) {
			defer log.RecordPanic()
			err := cr.checkFileFor(ctx, s, files, heavyCheck, missingMap, pg)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Errorf(Tr("error.check.failed"), s, err)
			}
			select {
			case done <- err == nil:
			case <-ctx.Done():
			}
		}(s)
	}
	goodCount := 0
	for i := len(cr.storages); i > 0; i-- {
		select {
		case ok := <-done:
			if ok {
				goodCount++
			}
		case <-ctx.Done():
			log.Warn(Tr("warn.sync.interrupted"))
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if goodCount == 0 {
		return nil, errors.New("All storages are failed")
	}
	return missingMap.RawMap(), nil
}

func (cr *Cluster) SetFilesetByExists(ctx context.Context, files []FileInfo) error {
	pg := mpb.New(mpb.WithRefreshRate(time.Second/2), mpb.WithAutoRefresh(), mpb.WithWidth(140))
	defer pg.Shutdown()
	log.SetLogOutput(pg)
	defer log.SetLogOutput(nil)

	missingMap, err := cr.CheckFiles(ctx, files, false, pg)
	if err != nil {
		return err
	}
	fileset := make(map[string]int64, len(files))
	stoCount := len(cr.storages)
	for _, f := range files {
		if t, ok := missingMap[f.Hash]; !ok || len(t.targets) < stoCount {
			fileset[f.Hash] = f.Size
		}
	}

	cr.mux.Lock()
	cr.fileset = fileset
	cr.mux.Unlock()
	return nil
}

func (cr *Cluster) syncFiles(ctx context.Context, files []FileInfo, heavyCheck bool) error {
	pg := mpb.New(mpb.WithRefreshRate(time.Second/2), mpb.WithAutoRefresh(), mpb.WithWidth(140))
	defer pg.Shutdown()
	log.SetLogOutput(pg)
	defer log.SetLogOutput(nil)

	cr.syncProg.Store(0)
	cr.syncTotal.Store(-1)

	missingMap, err := cr.CheckFiles(ctx, files, heavyCheck, pg)
	if err != nil {
		return err
	}
	var (
		missing           = make([]*fileInfoWithTargets, 0, len(missingMap))
		missingSize int64 = 0
	)
	for _, f := range missingMap {
		missing = append(missing, f)
		missingSize += f.Size
	}
	totalFiles := len(missing)
	if totalFiles == 0 {
		log.Info(Tr("info.sync.none"))
		return nil
	}

	go cr.notifyManager.OnSyncBegin(len(missing), missingSize)
	defer func() {
		go cr.notifyManager.OnSyncDone()
	}()

	cr.syncTotal.Store((int64)(totalFiles))

	ccfg, err := cr.GetConfig(ctx)
	if err != nil {
		return err
	}
	syncCfg := ccfg.Sync
	log.Infof(Tr("info.sync.config"), syncCfg)

	var stats syncStats
	stats.pg = pg
	stats.noOpen = syncCfg.Source == "center"
	stats.slots = limited.NewBufSlots(syncCfg.Concurrency + 1)
	stats.totalFiles = totalFiles
	for _, f := range missing {
		stats.totalSize += f.Size
	}

	var barUnit decor.SizeB1024
	stats.lastInc.Store(time.Now().UnixNano())
	stats.totalBar = pg.AddBar(stats.totalSize,
		mpb.BarRemoveOnComplete(),
		mpb.BarPriority(stats.slots.Cap()),
		mpb.PrependDecorators(
			decor.Name(Tr("hint.sync.total")),
			decor.NewPercentage("%.2f"),
		),
		mpb.AppendDecorators(
			decor.Any(func(decor.Statistics) string {
				return fmt.Sprintf("(%d + %d / %d) ", stats.okCount.Load(), stats.failCount.Load(), stats.totalFiles)
			}),
			decor.Counters(barUnit, "(%.1f/%.1f) "),
			decor.EwmaSpeed(barUnit, "%.1f ", 30),
			decor.OnComplete(
				decor.EwmaETA(decor.ET_STYLE_GO, 30), "done",
			),
		),
	)

	log.Infof(Tr("hint.sync.start"), totalFiles, utils.BytesToUnit((float64)(stats.totalSize)))
	start := time.Now()

	done := make(chan []storage.Storage, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	aliveStorages := len(cr.storages)
	for _, s := range cr.storages {
		tctx, cancel := context.WithTimeout(ctx, time.Second*10)
		err := s.CheckUpload(tctx)
		cancel()
		if err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			aliveStorages--
			log.Errorf("Storage %s does not work: %v", s.String(), err)
		}
	}
	if aliveStorages == 0 {
		err := errors.New("All storages are broken")
		log.Errorf(Tr("error.sync.failed"), err)
		return err
	}
	if aliveStorages < len(cr.storages) {
		log.Errorf(Tr("error.sync.part.working"), aliveStorages < len(cr.storages))
		select {
		case <-time.After(time.Minute):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	for _, f := range missing {
		log.Debugf("File %s is for %v", f.Hash, f.targets)
		pathRes, err := cr.fetchFile(ctx, &stats, f.FileInfo)
		if err != nil {
			log.Warn(Tr("warn.sync.interrupted"))
			return err
		}
		go func(f *fileInfoWithTargets, pathRes <-chan string) {
			defer log.RecordPanic()
			select {
			case path := <-pathRes:
				cr.syncProg.Add(1)
				if path == "" {
					select {
					case done <- nil: // TODO: or all storage?
					case <-ctx.Done():
					}
					return
				}
				defer os.Remove(path)
				// acquire slot here
				slotId, buf, free := stats.slots.Alloc(ctx)
				if buf == nil {
					return
				}
				defer free()
				_ = slotId
				var srcFd *os.File
				if srcFd, err = os.Open(path); err != nil {
					return
				}
				defer srcFd.Close()
				var failed []storage.Storage
				for _, target := range f.targets {
					if _, err = srcFd.Seek(0, io.SeekStart); err != nil {
						log.Errorf("Cannot seek file %q to start: %v", path, err)
						continue
					}
					if err = target.Create(f.Hash, srcFd); err != nil {
						failed = append(failed, target)
						log.Errorf(Tr("error.sync.create.failed"), target.String(), f.Hash, err)
						continue
					}
				}
				free()
				srcFd.Close()
				os.Remove(path)
				select {
				case done <- failed:
				case <-ctx.Done():
				}
			case <-ctx.Done():
				return
			}
		}(f, pathRes)
	}

	stLen := len(cr.storages)
	broken := make(map[storage.Storage]bool, stLen)

	for i := len(missing); i > 0; i-- {
		select {
		case failed := <-done:
			for _, s := range failed {
				if !broken[s] {
					broken[s] = true
					log.Debugf("Broken storage %d / %d", len(broken), stLen)
					if len(broken) >= stLen {
						cancel()
						err := errors.New("All storages are broken")
						log.Errorf(Tr("error.sync.failed"), err)
						return err
					}
				}
			}
		case <-ctx.Done():
			log.Warn(Tr("warn.sync.interrupted"))
			return ctx.Err()
		}
	}

	use := time.Since(start)
	stats.totalBar.Abort(true)
	pg.Wait()

	log.Infof(Tr("hint.sync.done"), use, utils.BytesToUnit((float64)(stats.totalSize)/use.Seconds()))
	return nil
}

func (cr *Cluster) Gc() {
	for _, s := range cr.storages {
		cr.gcFor(s)
	}
}

func (cr *Cluster) gcFor(s storage.Storage) {
	log.Infof(Tr("info.gc.start"), s.String())
	err := s.WalkDir(func(hash string, _ int64) error {
		if cr.issync.Load() {
			return context.Canceled
		}
		if _, ok := cr.CachedFileSize(hash); !ok {
			log.Infof(Tr("info.gc.found"), s.String()+"/"+hash)
			s.Remove(hash)
		}
		return nil
	})
	if err != nil {
		if err == context.Canceled {
			log.Warnf(Tr("warn.gc.interrupted"), s.String())
		} else {
			log.Errorf(Tr("error.gc.error"), err)
		}
		return
	}
	log.Infof(Tr("info.gc.done"), s.String())
}

func (cr *Cluster) fetchFile(ctx context.Context, stats *syncStats, f FileInfo) (<-chan string, error) {
	const (
		maxRetryCount  = 5
		maxTryWithOpen = 3
	)

	slotId, buf, free := stats.slots.Alloc(ctx)
	if buf == nil {
		return nil, ctx.Err()
	}

	pathRes := make(chan string, 1)
	go func() {
		defer log.RecordPanic()
		defer free()
		defer close(pathRes)

		var barUnit decor.SizeB1024
		var tried atomic.Int32
		tried.Store(1)
		bar := stats.pg.AddBar(f.Size,
			mpb.BarRemoveOnComplete(),
			mpb.BarPriority(slotId),
			mpb.PrependDecorators(
				decor.Name(Tr("hint.sync.downloading")),
				decor.Any(func(decor.Statistics) string {
					tc := tried.Load()
					if tc <= 1 {
						return ""
					}
					return fmt.Sprintf("(%d/%d) ", tc, maxRetryCount)
				}),
				decor.Name(f.Path, decor.WCSyncSpaceR),
			),
			mpb.AppendDecorators(
				decor.NewPercentage("%d", decor.WCSyncSpace),
				decor.Counters(barUnit, "[%.1f / %.1f]", decor.WCSyncSpace),
				decor.EwmaSpeed(barUnit, "%.1f", 30, decor.WCSyncSpace),
				decor.OnComplete(
					decor.EwmaETA(decor.ET_STYLE_GO, 30, decor.WCSyncSpace), "done",
				),
			),
		)
		defer bar.Abort(true)

		noOpen := stats.noOpen
		badOpen := false
		interval := time.Second
		for {
			bar.SetCurrent(0)
			hashMethod, err := getHashMethod(len(f.Hash))
			if err == nil {
				var path string
				if path, err = cr.fetchFileWithBuf(ctx, f, hashMethod, buf, noOpen, badOpen, func(r io.Reader) io.Reader {
					return ProxyReader(r, bar, stats.totalBar, &stats.lastInc)
				}); err == nil {
					pathRes <- path
					stats.okCount.Add(1)
					log.Infof(Tr("info.sync.downloaded"), f.Path,
						utils.BytesToUnit((float64)(f.Size)),
						(float64)(stats.totalBar.Current())/(float64)(stats.totalSize)*100)
					return
				}
			}
			bar.SetRefill(bar.Current())

			c := tried.Add(1)
			if c > maxRetryCount {
				log.Errorf(Tr("error.sync.download.failed"), f.Path, err)
				break
			}
			if c > maxTryWithOpen {
				badOpen = true
			}
			log.Errorf(Tr("error.sync.download.failed.retry"), f.Path, interval, err)
			select {
			case <-time.After(interval):
				interval *= 2
			case <-ctx.Done():
				return
			}
		}
		stats.failCount.Add(1)
	}()
	return pathRes, nil
}

func (cr *Cluster) fetchFileWithBuf(
	ctx context.Context, f FileInfo,
	hashMethod crypto.Hash, buf []byte,
	noOpen bool, badOpen bool,
	wrapper func(io.Reader) io.Reader,
) (path string, err error) {
	var (
		reqPath = f.Path
		query   url.Values
		req     *http.Request
		res     *http.Response
		fd      *os.File
		r       io.Reader
	)
	if badOpen {
		reqPath = "/openbmclapi/download/" + f.Hash
	} else if noOpen {
		query = url.Values{
			"noopen": {"1"},
		}
	}
	if req, err = cr.makeReqWithAuth(ctx, http.MethodGet, reqPath, query); err != nil {
		return
	}
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	if res, err = cr.client.Do(req); err != nil {
		return
	}
	defer res.Body.Close()
	if err = ctx.Err(); err != nil {
		return
	}
	if res.StatusCode != http.StatusOK {
		err = ErrorFromRedirect(utils.NewHTTPStatusErrorFromResponse(res), res)
		return
	}
	switch ce := strings.ToLower(res.Header.Get("Content-Encoding")); ce {
	case "":
		r = res.Body
	case "gzip":
		if r, err = gzip.NewReader(res.Body); err != nil {
			err = ErrorFromRedirect(err, res)
			return
		}
	case "deflate":
		if r, err = zlib.NewReader(res.Body); err != nil {
			err = ErrorFromRedirect(err, res)
			return
		}
	default:
		err = ErrorFromRedirect(fmt.Errorf("Unexpected Content-Encoding %q", ce), res)
		return
	}
	if wrapper != nil {
		r = wrapper(r)
	}

	hw := hashMethod.New()

	if fd, err = os.CreateTemp("", "*.downloading"); err != nil {
		return
	}
	path = fd.Name()
	defer func(path string) {
		if err != nil {
			os.Remove(path)
		}
	}(path)

	_, err = io.CopyBuffer(io.MultiWriter(hw, fd), r, buf)
	stat, err2 := fd.Stat()
	fd.Close()
	if err != nil {
		err = ErrorFromRedirect(err, res)
		return
	}
	if err2 != nil {
		err = err2
		return
	}
	if t := stat.Size(); f.Size >= 0 && t != f.Size {
		err = ErrorFromRedirect(fmt.Errorf("File size wrong, got %d, expect %d", t, f.Size), res)
		return
	} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != f.Hash {
		err = ErrorFromRedirect(fmt.Errorf("File hash not match, got %s, expect %s", hs, f.Hash), res)
		return
	}
	return
}

type downloadingItem struct {
	err  error
	done chan struct{}
}

func (cr *Cluster) lockDownloading(target string) (*downloadingItem, bool) {
	cr.downloadMux.RLock()
	item := cr.downloading[target]
	cr.downloadMux.RUnlock()
	if item != nil {
		return item, true
	}

	cr.downloadMux.Lock()
	defer cr.downloadMux.Unlock()

	if item = cr.downloading[target]; item != nil {
		return item, true
	}
	item = &downloadingItem{
		done: make(chan struct{}, 0),
	}
	cr.downloading[target] = item
	return item, false
}

func (cr *Cluster) DownloadFile(ctx context.Context, hash string) (err error) {
	hashMethod, err := getHashMethod(len(hash))
	if err != nil {
		return
	}

	f := FileInfo{
		Path:  "/openbmclapi/download/" + hash,
		Hash:  hash,
		Size:  -1,
		Mtime: 0,
	}
	item, ok := cr.lockDownloading(hash)
	if !ok {
		go func() {
			defer log.RecoverPanic(nil)
			var err error
			defer func() {
				if err != nil {
					log.Errorf(Tr("error.sync.download.failed"), hash, err)
				}
				item.err = err
				close(item.done)

				cr.downloadMux.Lock()
				defer cr.downloadMux.Unlock()
				delete(cr.downloading, hash)
			}()

			log.Infof(Tr("hint.sync.downloading.handler"), hash)

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				if cr.enabled.Load() {
					select {
					case <-cr.Disabled():
						cancel()
					case <-ctx.Done():
					}
				} else {
					select {
					case <-cr.WaitForEnable():
						cancel()
					case <-ctx.Done():
					}
				}
			}()
			defer cancel()

			var buf []byte
			_, buf, free := cr.allocBuf(ctx)
			if buf == nil {
				err = ctx.Err()
				return
			}
			defer free()

			path, err := cr.fetchFileWithBuf(ctx, f, hashMethod, buf, true, true, nil)
			if err != nil {
				return
			}
			defer os.Remove(path)
			var srcFd *os.File
			if srcFd, err = os.Open(path); err != nil {
				return
			}
			defer srcFd.Close()
			var stat os.FileInfo
			if stat, err = srcFd.Stat(); err != nil {
				return
			}
			size := stat.Size()

			for _, target := range cr.storages {
				if _, err = srcFd.Seek(0, io.SeekStart); err != nil {
					log.Errorf("Cannot seek file %q: %v", path, err)
					return
				}
				if err := target.Create(hash, srcFd); err != nil {
					log.Errorf(Tr("error.sync.create.failed"), target.String(), hash, err)
					continue
				}
			}

			cr.filesetMux.Lock()
			cr.fileset[hash] = -size // negative means that the file was not stored into the database yet
			cr.filesetMux.Unlock()
		}()
	}
	select {
	case <-item.done:
		err = item.err
	case <-ctx.Done():
		err = ctx.Err()
	case <-cr.Disabled():
		err = context.Canceled
	}
	return
}

func (cr *Cluster) checkUpdate() (err error) {
	if update.CurrentBuildTag == nil {
		return
	}
	log.Info(Tr("info.update.checking"))
	release, err := update.Check(cr.cachedCli, config.GithubAPI.Authorization)
	if err != nil || release == nil {
		return
	}
	// TODO: print all middle change logs
	log.Infof(Tr("info.update.detected"), release.Tag, update.CurrentBuildTag)
	log.Infof(Tr("info.update.changelog"), update.CurrentBuildTag, release.Tag, release.Body)
	cr.notifyManager.OnUpdateAvaliable(release)
	return
}
