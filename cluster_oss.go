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
	"os"
	"path/filepath"
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

	for _, f := range missing {
		logDebugf("File %s is for %v", f.Hash, f.targets)
		pathRes, err := cr.fetchFile(ctx, &stats, f.FileInfo)
		if err != nil {
			logWarn("File sync interrupted")
			return err
		}
		go func(f *fileInfoWithTargets) {
			select {
			case path := <-pathRes:
				if path != "" {
					defer os.Remove(path)
					for _, target := range f.targets {
						if err := copyFile(path, target, 0644); err != nil {
							logErrorf("Could not copy file %q to %q:\n\t%v", path, target, err)
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
		case <-stats.slots:
		case <-ctx.Done():
			logWarn("File sync interrupted")
			return ctx.Err()
		}
	}

	use := time.Since(start)
	logInfof("All files was synchronized, use time: %v, %s/s", use, bytesToUnit(stats.totalsize/use.Seconds()))
	return nil
}
