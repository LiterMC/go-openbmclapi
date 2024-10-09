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
	Cluster *Cluster
	Path    string
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
	res, err := cr.client.DoUseCache(req)
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
			ff.URLs[req.URL.String()] = RequestPath{
				Request: req,
				Cluster: cr,
				Path:    f.Path,
			}
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

	totalFiles := len(missingMap)

	var stats syncStats
	stats.pg = pg
	stats.slots = limited.NewBufSlots(slots)
	stats.totalFiles = totalFiles
	for _, f := range missingMap {
		stats.totalSize += f.Size
	}

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
		fileRes, err := c.fetchFile(ctx, &stats, info)
		if err != nil {
			log.TrWarnf("warn.sync.interrupted")
			return err
		}
		go func(info *StorageFileInfo, fileRes <-chan *os.File) {
			defer log.RecordPanic()
			select {
			case srcFd := <-fileRes:
				// cr.syncProg.Add(1)
				if srcFd == nil {
					select {
					case done <- nil: // TODO: or all storage?
					case <-ctx.Done():
					}
					return
				}
				defer os.Remove(srcFd.Name())
				defer srcFd.Close()
				// acquire slot here
				slotId, buf, free := stats.slots.Alloc(ctx)
				if buf == nil {
					return
				}
				defer free()
				_ = slotId
				var failed []storage.Storage
				for _, target := range info.Storages {
					if _, err = srcFd.Seek(0, io.SeekStart); err != nil {
						log.Errorf("Cannot seek file %q to start: %v", srcFd.Name(), err)
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
				os.Remove(srcFd.Name())
				select {
				case done <- failed:
				case <-ctx.Done():
				}
			case <-ctx.Done():
				return
			}
		}(info, fileRes)
	}

	for range len(missingMap) {
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

func (c *HTTPClient) fetchFile(ctx context.Context, stats *syncStats, f *StorageFileInfo) (<-chan *os.File, error) {
	const maxRetryCount = 10

	slotId, buf, free := stats.slots.Alloc(ctx)
	if buf == nil {
		return nil, ctx.Err()
	}

	hashMethod, err := getHashMethod(len(f.Hash))
	if err != nil {
		return nil, err
	}

	reqInd := 0
	reqs := make([]RequestPath, 0, len(f.URLs))
	for _, rq := range f.URLs {
		reqs = append(reqs, rq)
	}

	fileRes := make(chan *os.File, 1)
	go func() {
		defer log.RecordPanic()
		defer free()
		defer close(fileRes)

		var barUnit decor.SizeB1024
		var tried atomic.Int32
		tried.Store(1)

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
				decor.Name(f.Hash, decor.WCSyncSpaceR),
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

		fd, err := os.CreateTemp("", "*.downloading")
		if err != nil {
			log.Errorf("Cannot create temporary file: %s", err)
			stats.failCount.Add(1)
			return
		}
		successed := false
		defer func(fd *os.File) {
			if !successed {
				fd.Close()
				os.Remove(fd.Name())
			}
		}(fd)
		// prealloc space
		if err := fd.Truncate(f.Size); err != nil {
			log.Warnf("File space pre-alloc failed: %v", err)
		}

		downloadOnce := func() error {
			if _, err := fd.Seek(io.SeekStart, 0); err != nil {
				return err
			}
			rp := reqs[reqInd]
			if err := c.fetchFileWithBuf(ctx, rp.Request, f.Size, hashMethod, f.Hash, fd, buf, func(r io.Reader) io.Reader {
				return utils.ProxyPBReader(r, bar, stats.totalBar, &stats.lastInc)
			}); err != nil {
				reqInd = (reqInd + 1) % len(reqs)
				var rerr *utils.RedirectError
				if errors.As(err, &rerr) {
					go func() {
						if err := rp.Cluster.ReportDownload(context.WithoutCancel(ctx), rerr.GetResponse(), rerr.Unwrap()); err != nil {
							log.Warnf("Report API error: %v", err)
						}
					}()
				}
				return err
			}
			return nil
		}

		interval := time.Second
		for {
			bar.SetCurrent(0)
			err := downloadOnce()
			if err == nil {
				break
			}
			bar.SetRefill(bar.Current())

			c := tried.Add(1)
			if c > maxRetryCount || errors.Is(err, context.Canceled) {
				log.TrErrorf("error.sync.download.failed", f.Hash, err)
				stats.failCount.Add(1)
				return
			}
			log.TrErrorf("error.sync.download.failed.retry", f.Hash, interval, err)
			select {
			case <-time.After(interval):
				interval = min(interval*2, time.Minute*10)
			case <-ctx.Done():
				stats.failCount.Add(1)
				return
			}
		}
		successed = true
		fileRes <- fd
		stats.okCount.Add(1)
		log.Infof(lang.Tr("info.sync.downloaded"), f.Hash,
			utils.BytesToUnit((float64)(f.Size)),
			(float64)(stats.totalBar.Current())/(float64)(stats.totalSize)*100)
	}()
	return fileRes, nil
}

func (c *HTTPClient) fetchFileWithBuf(
	ctx context.Context, req *http.Request,
	size int64, hashMethod crypto.Hash, hash string,
	rw io.ReadWriteSeeker, buf []byte,
	wrapper func(io.Reader) io.Reader,
) (err error) {
	var (
		res *http.Response
		r   io.Reader
	)
	req = req.Clone(ctx)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	if res, err = c.Do(req); err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		err = utils.NewHTTPStatusErrorFromResponse(res)
	} else {
		switch ce := strings.ToLower(res.Header.Get("Content-Encoding")); ce {
		case "":
			r = res.Body
			if res.ContentLength >= 0 && res.ContentLength != size {
				err = fmt.Errorf("File size wrong, got %d, expect %d", res.ContentLength, size)
			}
		case "gzip":
			r, err = gzip.NewReader(res.Body)
		case "deflate":
			r, err = zlib.NewReader(res.Body)
		default:
			err = fmt.Errorf("Unexpected Content-Encoding %q", ce)
		}
	}
	if err != nil {
		return utils.ErrorFromRedirect(err, res)
	}
	if wrapper != nil {
		r = wrapper(r)
	}

	if n, err := io.CopyBuffer(rw, r, buf); err != nil {
		return utils.ErrorFromRedirect(err, res)
	} else if n != size {
		return utils.ErrorFromRedirect(fmt.Errorf("File size wrong, got %d, expect %d", n, size), res)
	}
	if _, err := rw.Seek(io.SeekStart, 0); err != nil {
		return err
	}
	hw := hashMethod.New()
	if _, err := io.CopyBuffer(hw, rw, buf); err != nil {
		return err
	}
	if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != hash {
		return utils.ErrorFromRedirect(fmt.Errorf("File hash not match, got %s, expect %s", hs, hash), res)
	}
	return
}

func (c *HTTPClient) Gc(
	ctx context.Context,
	manager *storage.Manager,
	files map[string]*StorageFileInfo,
) error {
	errs := make([]error, len(manager.Storages))
	var wg sync.WaitGroup
	for i, s := range manager.Storages {
		wg.Add(1)
		go func(i int, s storage.Storage) {
			defer wg.Done()
			errs[i] = s.WalkDir(func(hash string, size int64) error {
				info, ok := files[hash]
				ok = ok && slices.Contains(info.Storages, s)
				if !ok {
					s.Remove(hash)
				}
				return nil
			})
		}(i, s)
	}
	wg.Wait()
	return errors.Join(errs...)
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
