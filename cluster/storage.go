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
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hamba/avro/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/LiterMC/go-openbmclapi/lang"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/utils"
)

// from <https://github.com/bangbang93/openbmclapi/blob/master/src/constants.ts>
var fileListSchema = avro.MustParse(`{
	"type": "array",
	"items": {
		"type": "record",
		"name": "fileinfo",
		"fields": [
			{"name": "path", "type": "string"},
			{"name": "hash", "type": "string"},
			{"name": "size", "type": "long"},
			{"name": "mtime", "type": "long"}
		]
	}
}`)

type FileInfo struct {
	Path  string `json:"path" avro:"path"`
	Hash  string `json:"hash" avro:"hash"`
	Size  int64  `json:"size" avro:"size"`
	Mtime int64  `json:"mtime" avro:"mtime"`
}

type RequestPath struct {
	*http.Request
	Path string
}

type StorageFileInfo struct {
	Hash     string
	Size     int64
	Storages []storage.Storage
	URLs     map[string]RequestPath
}

func (cr *Cluster) GetFileList(ctx context.Context, fileMap map[string]*StorageFileInfo, forceAll bool) error {
	var query url.Values
	lastMod := cr.fileListLastMod
	if forceAll {
		lastMod = 0
	}
	if lastMod > 0 {
		query = url.Values{
			"lastModified": {strconv.FormatInt(lastMod, 10)},
		}
	}
	req, err := cr.makeReqWithAuth(ctx, http.MethodGet, "/openbmclapi/files", query)
	if err != nil {
		return err
	}
	res, err := cr.cachedCli.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		//
	case http.StatusNoContent, http.StatusNotModified:
		return nil
	default:
		return utils.NewHTTPStatusErrorFromResponse(res)
	}
	log.Debug("Parsing filelist body ...")
	zr, err := zstd.NewReader(res.Body)
	if err != nil {
		return err
	}
	defer zr.Close()
	var files []FileInfo
	if err := avro.NewDecoderForSchema(fileListSchema, zr).Decode(&files); err != nil {
		return err
	}

	for _, f := range files {
		if f.Mtime > lastMod {
			lastMod = f.Mtime
		}
		if ff, ok := fileMap[f.Hash]; ok {
			if ff.Size != f.Size {
				log.Panicf("Hash conflict detected, hash of both %q (%dB) and %v (%dB) is %s", f.Path, f.Size, ff.URLs, ff.Size, f.Hash)
			}
			for _, s := range cr.storages {
				sto := cr.storageManager.Storages[s]
				if i, ok := slices.BinarySearchFunc(ff.Storages, sto, storageIdSortFunc); !ok {
					ff.Storages = slices.Insert(ff.Storages, i, sto)
				}
			}
		} else {
			ff := &StorageFileInfo{
				Hash:     f.Hash,
				Size:     f.Size,
				Storages: make([]storage.Storage, len(cr.storages)),
				URLs:     make(map[string]RequestPath),
			}
			for i, s := range cr.storages {
				ff.Storages[i] = cr.storageManager.Storages[s]
			}
			slices.SortFunc(ff.Storages, storageIdSortFunc)
			req, err := cr.makeReqWithAuth(context.Background(), http.MethodGet, f.Path, nil)
			if err != nil {
				return err
			}
			ff.URLs[req.URL.String()] = RequestPath{Request: req, Path: f.Path}
			fileMap[f.Hash] = ff
		}
	}
	cr.fileListLastMod = lastMod
	log.Debugf("Filelist parsed, length = %d, lastMod = %d", len(files), lastMod)
	return nil
}

func storageIdSortFunc(a, b storage.Storage) int {
	if a.Id() < b.Id() {
		return -1
	}
	return 1
}

// func SyncFiles(ctx context.Context, manager *storage.Manager, files map[string]*StorageFileInfo, heavyCheck bool) bool {
// 	log.TrInfof("info.sync.prepare", len(files))

// 	slices.SortFunc(files, func(a, b *StorageFileInfo) int { return a.Size - b.Size })
// 	if cr.syncFiles(ctx, files, heavyCheck) != nil {
// 		return false
// 	}

// 	cr.filesetMux.Lock()
// 	for _, f := range files {
// 		cr.fileset[f.Hash] = f.Size
// 	}
// 	cr.filesetMux.Unlock()

// 	return true
// }

var emptyStr string

func checkFile(
	ctx context.Context,
	manager *storage.Manager,
	files map[string]*StorageFileInfo,
	heavy bool,
	missing map[string]*StorageFileInfo,
	pg *mpb.Progress,
) (err error) {
	var missingCount atomic.Int32
	addMissing := func(f *StorageFileInfo, sto storage.Storage) {
		missingCount.Add(1)
		if info, ok := missing[f.Hash]; ok {
			info.Storages = append(info.Storages, sto)
		} else {
			info := new(StorageFileInfo)
			*info = *f
			info.Storages = []storage.Storage{sto}
			missing[f.Hash] = info
		}
	}

	log.TrInfof("info.check.start", heavy)

	var (
		checkingHash     atomic.Pointer[string]
		lastCheckingHash string
		slots            *limited.BufSlots
		wg               sync.WaitGroup
	)
	checkingHash.Store(&emptyStr)

	if heavy {
		slots = limited.NewBufSlots(runtime.GOMAXPROCS(0) * 2)
	}

	bar := pg.AddBar(0,
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name(lang.Tr("hint.check.checking")),
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
			lastCheckingHash = *checkingHash.Load()
			if lastCheckingHash != "" {
				_, err = fmt.Fprintln(w, "\t", lastCheckingHash)
			}
			return
		}), false),
	)
	defer bar.Wait()
	defer bar.Abort(true)

	bar.SetTotal(0x100, false)

	ssizeMap := make(map[storage.Storage]map[string]int64, len(manager.Storages))
	for _, sto := range manager.Storages {
		sizeMap := make(map[string]int64, len(files))
		ssizeMap[sto] = sizeMap
		wg.Add(1)
		go func(sto storage.Storage, sizeMap map[string]int64) {
			defer wg.Done()
			start := time.Now()
			var checkedMp [256]bool
			if err := sto.WalkDir(func(hash string, size int64) error {
				if n := utils.HexTo256(hash); !checkedMp[n] {
					checkedMp[n] = true
					now := time.Now()
					bar.EwmaIncrement(now.Sub(start))
					start = now
				}
				sizeMap[hash] = size
				return nil
			}); err != nil {
				log.Errorf("Cannot walk %s: %v", sto.Id(), err)
				return
			}
		}(sto, sizeMap)
	}
	wg.Wait()

	bar.SetCurrent(0)
	bar.SetTotal((int64)(len(files)), false)
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		start := time.Now()
		hash := f.Hash
		checkingHash.Store(&hash)
		if f.Size == 0 {
			log.Debugf("Skipped empty file %s", hash)
			bar.EwmaIncrement(time.Since(start))
			continue
		}
		for _, sto := range f.Storages {
			name := sto.Id() + "/" + hash
			size, ok := ssizeMap[sto][hash]
			if !ok {
				// log.Debugf("Could not found file %q", name)
				addMissing(f, sto)
				bar.EwmaIncrement(time.Since(start))
				continue
			}
			if size != f.Size {
				log.TrWarnf("warn.check.modified.size", name, size, f.Size)
				addMissing(f, sto)
				bar.EwmaIncrement(time.Since(start))
				continue
			}
			if !heavy {
				bar.EwmaIncrement(time.Since(start))
				continue
			}
			hashMethod, err := getHashMethod(len(hash))
			if err != nil {
				log.TrErrorf("error.check.unknown.hash.method", hash)
				bar.EwmaIncrement(time.Since(start))
				continue
			}
			_, buf, free := slots.Alloc(ctx)
			if buf == nil {
				return ctx.Err()
			}
			wg.Add(1)
			go func(f *StorageFileInfo, buf []byte, free func()) {
				defer log.RecoverPanic(nil)
				defer wg.Done()
				miss := true
				r, err := sto.Open(hash)
				if err != nil {
					log.TrErrorf("error.check.open.failed", name, err)
				} else {
					hw := hashMethod.New()
					_, err = io.CopyBuffer(hw, r, buf[:])
					r.Close()
					if err != nil {
						log.TrErrorf("error.check.hash.failed", name, err)
					} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != hash {
						log.TrWarnf("warn.check.modified.hash", name, hs, hash)
					} else {
						miss = false
					}
				}
				bar.EwmaIncrement(time.Since(start))
				free()
				if miss {
					addMissing(f, sto)
				}
			}(f, buf, free)
		}
	}
	wg.Wait()

	checkingHash.Store(&emptyStr)

	bar.SetTotal(-1, true)
	log.TrInfof("info.check.done", missingCount.Load())
	return nil
}

type syncStats struct {
	slots *limited.BufSlots

	totalSize          int64
	okCount, failCount atomic.Int32
	totalFiles         int

	pg       *mpb.Progress
	totalBar *mpb.Bar
	lastInc  atomic.Int64
}

func (c *HTTPClient) SyncFiles(
	ctx context.Context,
	manager *storage.Manager,
	files map[string]*StorageFileInfo,
	heavy bool,
	slots int,
) error {
	pg := mpb.New(mpb.WithRefreshRate(time.Second/2), mpb.WithAutoRefresh(), mpb.WithWidth(140))
	defer pg.Shutdown()
	log.SetLogOutput(pg)
	defer log.SetLogOutput(nil)

	missingMap := make(map[string]*StorageFileInfo)
	if err := checkFile(ctx, manager, files, heavy, missingMap, pg); err != nil {
		return err
	}

	totalFiles := len(files)

	var stats syncStats
	stats.pg = pg
	stats.slots = limited.NewBufSlots(slots)
	stats.totalFiles = totalFiles

	var barUnit decor.SizeB1024
	stats.lastInc.Store(time.Now().UnixNano())
	stats.totalBar = pg.AddBar(stats.totalSize,
		mpb.BarRemoveOnComplete(),
		mpb.BarPriority(stats.slots.Cap()),
		mpb.PrependDecorators(
			decor.Name(lang.Tr("hint.sync.total")),
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

	log.TrInfof("hint.sync.start", totalFiles, utils.BytesToUnit((float64)(stats.totalSize)))
	start := time.Now()

	done := make(chan []storage.Storage, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stLen := len(manager.Storages)
	aliveStorages := make(map[storage.Storage]struct{}, stLen)
	for _, s := range manager.Storages {
		tctx, cancel := context.WithTimeout(ctx, time.Second*10)
		err := s.CheckUpload(tctx)
		cancel()
		if err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			log.Errorf("Storage %s does not work: %v", s.String(), err)
		} else {
			aliveStorages[s] = struct{}{}
		}
	}
	if len(aliveStorages) == 0 {
		err := errors.New("All storages are broken")
		log.TrErrorf("error.sync.failed", err)
		return err
	}
	if len(aliveStorages) < stLen {
		log.TrErrorf("error.sync.part.working", len(aliveStorages), stLen)
		select {
		case <-time.After(time.Minute):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	for _, info := range missingMap {
		log.Debugf("File %s is for %s", info.Hash, joinStorageIDs(info.Storages))
		pathRes, err := c.fetchFile(ctx, &stats, info)
		if err != nil {
			log.TrWarnf("warn.sync.interrupted")
			return err
		}
		go func(info *StorageFileInfo, pathRes <-chan string) {
			defer log.RecordPanic()
			select {
			case path := <-pathRes:
				// cr.syncProg.Add(1)
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
				for _, target := range info.Storages {
					if _, err = srcFd.Seek(0, io.SeekStart); err != nil {
						log.Errorf("Cannot seek file %q to start: %v", path, err)
						continue
					}
					if err = target.Create(info.Hash, srcFd); err != nil {
						failed = append(failed, target)
						log.TrErrorf("error.sync.create.failed", target.String(), info.Hash, err)
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
		}(info, pathRes)
	}

	for i := len(missingMap); i > 0; i-- {
		select {
		case failed := <-done:
			for _, s := range failed {
				if _, ok := aliveStorages[s]; ok {
					delete(aliveStorages, s)
					log.Debugf("Broken storage %d / %d", stLen-len(aliveStorages), stLen)
					if len(aliveStorages) == 0 {
						cancel()
						err := errors.New("All storages are broken")
						log.TrErrorf("error.sync.failed", err)
						return err
					}
				}
			}
		case <-ctx.Done():
			log.TrWarnf("warn.sync.interrupted")
			return ctx.Err()
		}
	}

	use := time.Since(start)
	stats.totalBar.Abort(true)
	pg.Wait()

	log.TrInfof("hint.sync.done", use, utils.BytesToUnit((float64)(stats.totalSize)/use.Seconds()))
	return nil
}

func (c *HTTPClient) fetchFile(ctx context.Context, stats *syncStats, f *StorageFileInfo) (<-chan string, error) {
	const maxRetryCount = 10

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

		fPath := f.Hash // TODO: show downloading URL instead? Will it be too long?

		bar := stats.pg.AddBar(f.Size,
			mpb.BarRemoveOnComplete(),
			mpb.BarPriority(slotId),
			mpb.PrependDecorators(
				decor.Name(lang.Tr("hint.sync.downloading")),
				decor.Any(func(decor.Statistics) string {
					tc := tried.Load()
					if tc <= 1 {
						return ""
					}
					return fmt.Sprintf("(%d/%d) ", tc, maxRetryCount)
				}),
				decor.Name(fPath, decor.WCSyncSpaceR),
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

		interval := time.Second
		for {
			bar.SetCurrent(0)
			hashMethod, err := getHashMethod(len(f.Hash))
			if err == nil {
				var path string
				if path, err = c.fetchFileWithBuf(ctx, f, hashMethod, buf, func(r io.Reader) io.Reader {
					return utils.ProxyPBReader(r, bar, stats.totalBar, &stats.lastInc)
				}); err == nil {
					pathRes <- path
					stats.okCount.Add(1)
					log.Infof(lang.Tr("info.sync.downloaded"), fPath,
						utils.BytesToUnit((float64)(f.Size)),
						(float64)(stats.totalBar.Current())/(float64)(stats.totalSize)*100)
					return
				}
			}
			bar.SetRefill(bar.Current())

			c := tried.Add(1)
			if c > maxRetryCount {
				log.TrErrorf("error.sync.download.failed", fPath, err)
				break
			}
			log.TrErrorf("error.sync.download.failed.retry", fPath, interval, err)
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

func (c *HTTPClient) fetchFileWithBuf(
	ctx context.Context, f *StorageFileInfo,
	hashMethod crypto.Hash, buf []byte,
	wrapper func(io.Reader) io.Reader,
) (path string, err error) {
	var (
		req *http.Request
		res *http.Response
		fd  *os.File
		r   io.Reader
	)
	for _, rq := range f.URLs {
		req = rq.Request
		break
	}
	req = req.Clone(ctx)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	if res, err = c.Do(req); err != nil {
		return
	}
	defer res.Body.Close()
	if err = ctx.Err(); err != nil {
		return
	}
	if res.StatusCode != http.StatusOK {
		err = utils.ErrorFromRedirect(utils.NewHTTPStatusErrorFromResponse(res), res)
		return
	}
	switch ce := strings.ToLower(res.Header.Get("Content-Encoding")); ce {
	case "":
		r = res.Body
	case "gzip":
		if r, err = gzip.NewReader(res.Body); err != nil {
			err = utils.ErrorFromRedirect(err, res)
			return
		}
	case "deflate":
		if r, err = zlib.NewReader(res.Body); err != nil {
			err = utils.ErrorFromRedirect(err, res)
			return
		}
	default:
		err = utils.ErrorFromRedirect(fmt.Errorf("Unexpected Content-Encoding %q", ce), res)
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
		err = utils.ErrorFromRedirect(err, res)
		return
	}
	if err2 != nil {
		err = err2
		return
	}
	if t := stat.Size(); f.Size >= 0 && t != f.Size {
		err = utils.ErrorFromRedirect(fmt.Errorf("File size wrong, got %d, expect %d", t, f.Size), res)
		return
	} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != f.Hash {
		err = utils.ErrorFromRedirect(fmt.Errorf("File hash not match, got %s, expect %s", hs, f.Hash), res)
		return
	}
	return
}

func getHashMethod(l int) (hashMethod crypto.Hash, err error) {
	switch l {
	case 32:
		hashMethod = crypto.MD5
	case 40:
		hashMethod = crypto.SHA1
	default:
		err = fmt.Errorf("Unknown hash length %d", l)
	}
	return
}

func joinStorageIDs(storages []storage.Storage) string {
	ss := make([]string, len(storages))
	for i, s := range storages {
		ss[i] = s.Id()
	}
	return "[" + strings.Join(ss, ", ") + "]"
}
