/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type fileInfoWithTargets struct {
	FileInfo
	targets []string
}

func (cr *Cluster) ossSyncFiles(ctx context.Context, files []FileInfo) error {
	missingMap := make(map[string]*fileInfoWithTargets, 4)
	for _, item := range cr.ossList {
		dir := filepath.Join(item.FolderPath, "download")
		need := cr.CheckFiles(dir, files)
		for _, f := range need {
			if info := missingMap[f.Hash]; info != nil {
				info.targets = append(info.targets, dir)
			} else {
				missingMap[f.Hash] = &fileInfoWithTargets{
					FileInfo: f,
					targets:  []string{dir},
				}
			}
		}
	}

	missing := make([]*fileInfoWithTargets, 0, len(missingMap))
	for _, f := range missingMap {
		missing = append(missing, f)
	}

	fl := len(missing)
	if fl == 0 {
		logInfo("All oss files was synchronized")
		return nil
	}

	var stats syncStats
	stats.slots = make(chan []byte, cr.maxConn)
	stats.fl = fl
	for _, f := range missing {
		stats.totalsize += (float64)(f.Size)
	}
	for i := cap(stats.slots); i > 0; i-- {
		stats.slots <- make([]byte, 1024*1024)
	}

	logInfof("Starting sync files, count: %d, total: %s", fl, bytesToUnit(stats.totalsize))
	start := time.Now()

	done := make(chan struct{}, 1)

	for _, f := range missing {
		logDebugf("File %s is for %v", f.Hash, f.targets)
		pathRes, err := cr.fetchFile(ctx, &stats, f.FileInfo)
		if err != nil {
			logWarn("File sync interrupted")
			return err
		}
		go func(f *fileInfoWithTargets) {
			defer func() {
				done <- struct{}{}
			}()
			select {
			case path := <-pathRes:
				if path != "" {
					defer os.Remove(path)
					// acquire slot here
					buf := <-stats.slots
					defer func(){
						stats.slots <- buf
					}()
					var srcFd *os.File
					if srcFd, err = os.Open(path); err != nil {
						return
					}
					defer srcFd.Close()
					relpath := hashToFilename(f.Hash)
					for _, target := range f.targets {
						target = filepath.Join(target, relpath)
						dstFd, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
						if err != nil {
							logErrorf("Could not create %q: %v", target, err)
							continue
						}
						if _, err = srcFd.Seek(0, io.SeekStart); err != nil {
							logErrorf("Could not seek file %q: %v", path, err)
							continue
						}
						_, err = io.CopyBuffer(dstFd, srcFd, buf)
						dstFd.Close()
						if err != nil {
							logErrorf("Could not copy from %q to %q:\n\t%v", path, target, err)
							continue
						}
					}
				}
			case <-ctx.Done():
				return
			}
		}(f)
	}
	for i := cap(stats.slots); i > 0; i-- {
		select {
		case <-done:
		case <-ctx.Done():
			logWarn("File sync interrupted")
			return ctx.Err()
		}
	}

	use := time.Since(start)
	logInfof("All files was synchronized, use time: %v, %s/s", use, bytesToUnit(stats.totalsize/use.Seconds()))
	return nil
}

func createOssMirrorDir(item *OSSItem) {
	logInfof("Creating OSS folder %s", item.FolderPath)
	if err := os.MkdirAll(item.FolderPath, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", item.FolderPath, err)
		os.Exit(2)
	}

	// cacheDir := filepath.Join(baseDir, "cache")
	// downloadDir := filepath.Join(item.FolderPath, "download")
	// os.RemoveAll(downloadDir)
	// if err := os.Mkdir(downloadDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
	// 	logErrorf("Cannot create OSS mirror folder %q: %v", downloadDir, err)
	// 	os.Exit(2)
	// }
	// for i := 0; i < 0x100; i++ {
	// 	d := hex.EncodeToString([]byte{(byte)(i)})
	// 	o := filepath.Join(cacheDir, d)
	// 	t := filepath.Join(downloadDir, d)
	// 	os.Mkdir(o, 0755)
	// 	if err := os.Symlink(filepath.Join("..", "..", o), t); err != nil {
	// 		logErrorf("Cannot create OSS mirror cache symlink %q: %v", t, err)
	// 		os.Exit(2)
	// 	}
	// }

	if !item.SkipMeasureGen {
		logDebug("Creating measure files")
		measureDir := filepath.Join(item.FolderPath, "measure")
		if err := os.Mkdir(measureDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			logErrorf("Cannot create OSS mirror folder %q: %v", measureDir, err)
			os.Exit(2)
		}
		const chunkSize = 1024 * 1024
		var chunk [chunkSize]byte
		for i := 1; i <= 200; i++ {
			size := i * chunkSize
			t := filepath.Join(measureDir, strconv.Itoa(i))
			if stat, err := os.Stat(t); err == nil {
				x := stat.Size()
				if x == (int64)(size) {
					logDebug("Skipping", t)
					continue
				}
				logDebugf("File [%d] size %d does not match %d", i, x, size)
			} else {
				logDebugf("Cannot get stat of %s: %v", t, err)
			}
			logDebug("Writing", t)
			fd, err := os.Create(t)
			if err != nil {
				logErrorf("Cannot create OSS mirror measure file %q: %v", t, err)
				os.Exit(2)
			}
			for j := 0; j < i; j++ {
				if _, err = fd.Write(chunk[:]); err != nil {
					logErrorf("Cannot write OSS mirror measure file %q: %v", t, err)
					os.Exit(2)
				}
			}
			fd.Close()
		}
		logDebug("Measure files created")
	}
}

func checkOSS(ctx context.Context, client *http.Client, item *OSSItem, size int) (supportRange bool, err error) {
	targetSize := (int64)(size) * 1024 * 1024
	logInfof("Checking %s for %d bytes ...", item.RedirectBase, targetSize)

	target, err := url.JoinPath(item.RedirectBase, "measure", strconv.Itoa(size))
	if err != nil {
		return false, fmt.Errorf("Cannot check OSS server: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return
	}
	req.Header.Set("Range", "bytes=1-")
	res, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("OSS check request failed %q: %w", target, err)
	}
	defer res.Body.Close()
	logDebugf("OSS check response status code %d %s", res.StatusCode, res.Status)
	if supportRange = res.StatusCode == http.StatusPartialContent; supportRange {
		logDebug("OSS support Range header!")
		targetSize--
	} else if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("OSS check request failed %q: %d %s", target, res.StatusCode, res.Status)
	} else {
		crange := res.Header.Get("Content-Range")
		if len(crange) > 0 {
			logWarn("Non standard http response detected, responsed 'Content-Range' header with status 200, expected status 206")
			fields := strings.Fields(crange)
			if len(fields) >= 2 && fields[0] == "bytes" && strings.HasPrefix(fields[1], "1-") {
				logDebug("OSS support Range header?")
				supportRange = true
				targetSize--
			}
		}
	}
	logDebug("reading OSS response")
	start := time.Now()
	n, err := io.Copy(io.Discard, res.Body)
	if err != nil {
		return false, fmt.Errorf("OSS check request failed %q: %w", target, err)
	}
	used := time.Since(start)
	if n != targetSize {
		return false, fmt.Errorf("OSS check request failed %q: expected %d bytes, but got %d bytes", target, targetSize, n)
	}
	logInfof("Check finished for %q, used %v, %s/s; supportRange=%v", target, used, bytesToUnit((float64)(n)/used.Seconds()), supportRange)
	return
}
