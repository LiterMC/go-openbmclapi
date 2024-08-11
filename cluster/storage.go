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
	"context"
	"crypto"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strconv"
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

type StorageFileInfo struct {
	FileInfo
	Storages []storage.Storage
}

func (cr *Cluster) GetFileList(ctx context.Context, fileMap map[string]*StorageFileInfo, forceAll bool) (err error) {
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
		return
	}
	res, err := cr.cachedCli.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		//
	case http.StatusNoContent, http.StatusNotModified:
		return
	default:
		err = utils.NewHTTPStatusErrorFromResponse(res)
		return
	}
	log.Debug("Parsing filelist body ...")
	zr, err := zstd.NewReader(res.Body)
	if err != nil {
		return
	}
	defer zr.Close()
	var files []FileInfo
	if err = avro.NewDecoderForSchema(fileListSchema, zr).Decode(&files); err != nil {
		return
	}

	for _, f := range files {
		if f.Mtime > lastMod {
			lastMod = f.Mtime
		}
		if ff, ok := fileMap[f.Hash]; ok {
			if ff.Size != f.Size {
				log.Panicf("Hash conflict detected, hash of both %q (%dB) and %q (%dB) is %s", ff.Path, ff.Size, f.Path, f.Size, f.Hash)
			}
			for _, s := range cr.storages {
				sto := cr.storageManager.Storages[s]
				if i, ok := slices.BinarySearchFunc(ff.Storages, sto, storageIdSortFunc); !ok {
					ff.Storages = slices.Insert(ff.Storages, i, sto)
				}
			}
		} else {
			ff := &StorageFileInfo{
				FileInfo: f,
				Storages: make([]storage.Storage, len(cr.storages)),
			}
			for i, s := range cr.storages {
				ff.Storages[i] = cr.storageManager.Storages[s]
			}
			slices.SortFunc(ff.Storages, storageIdSortFunc)
			fileMap[f.Hash] = ff
		}
	}
	cr.fileListLastMod = lastMod
	log.Debugf("Filelist parsed, length = %d, lastMod = %d", len(files), lastMod)
	return
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
	addMissing := func(f FileInfo, sto storage.Storage) {
		missingCount.Add(1)
		if info, ok := missing[f.Hash]; ok {
			info.Storages = append(info.Storages, sto)
		} else {
			missing[f.Hash] = &StorageFileInfo{
				FileInfo: f,
				Storages: []storage.Storage{sto},
			}
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
				addMissing(f.FileInfo, sto)
				bar.EwmaIncrement(time.Since(start))
				continue
			}
			if size != f.Size {
				log.TrWarnf("warn.check.modified.size", name, size, f.Size)
				addMissing(f.FileInfo, sto)
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
			go func(f FileInfo, buf []byte, free func()) {
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
			}(f.FileInfo, buf, free)
		}
	}
	wg.Wait()

	checkingHash.Store(&emptyStr)

	bar.SetTotal(-1, true)
	log.TrInfof("info.check.done", missingCount.Load())
	return nil
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
