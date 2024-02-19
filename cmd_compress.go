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
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func cmdZipCache(args []string) {
	flagVerbose := false
	flagAll := false
	flagOverwrite := false
	flagKeep := false
	for _, a := range args {
		a = strings.ToLower(a)
		if len(a) > 0 && a[0] == '-' {
			a = a[1:]
			if len(a) > 0 && a[0] == '-' {
				a = a[1:]
			} else {
				for _, aa := range a {
					switch aa {
					case 'v':
						flagVerbose = true
					case 'a':
						flagAll = true
					case 'o':
						flagOverwrite = true
					case 'k':
						flagKeep = true
					default:
						fmt.Printf("Unknown option %q\n", aa)
						os.Exit(2)
					}
				}
				continue
			}
		}
		switch a {
		case "verbose", "v":
			flagVerbose = true
		case "all", "a":
			flagAll = true
		case "overwrite", "o":
			flagOverwrite = true
		case "keep", "k":
			flagKeep = true
		default:
			fmt.Printf("Unknown option %q\n", a)
			os.Exit(2)
		}
	}
	cacheDir := filepath.Join(baseDir, "cache")
	fmt.Printf("Cache directory = %q\n", cacheDir)
	err := walkCacheDir(cacheDir, func(hash string, _ int64) (_ error) {
		path := filepath.Join(cacheDir, hash[0:2], hash)
		if strings.HasSuffix(path, ".gz") {
			return
		}
		target := path + ".gz"
		if !flagOverwrite {
			if _, err := os.Stat(target); err == nil {
				return
			}
		}
		srcFd, err := os.Open(path)
		if err != nil {
			fmt.Printf("Error: could not open file %q: %v\n", path, err)
			return
		}
		defer srcFd.Close()
		stat, err := srcFd.Stat()
		if err != nil {
			fmt.Printf("Error: could not get stat of %q: %v\n", path, err)
			return
		}
		if flagAll || stat.Size() > 1024*10 {
			if flagVerbose {
				fmt.Printf("compressing %s\n", path)
			}
			tmpPath := target + ".tmp"
			var dstFd *os.File
			if dstFd, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
				fmt.Printf("Error: could not create %q: %v\n", tmpPath, err)
				return
			}
			defer dstFd.Close()
			w := gzip.NewWriter(dstFd)
			defer w.Close()

			if _, err = io.Copy(w, srcFd); err != nil {
				os.Remove(tmpPath)
				fmt.Printf("Error: could not compress %q: %v\n", path, err)
				return
			}
			os.Remove(target)
			if err = os.Rename(tmpPath, target); err != nil {
				os.Remove(tmpPath)
				fmt.Printf("Error: could not rename %q to %q\n", tmpPath, target)
				return
			}
			if !flagKeep {
				os.Remove(path)
			}
		}
		return
	})
	if err != nil {
		fmt.Printf("Could not walk cache directory: %v", err)
		os.Exit(1)
	}
}

func cmdUnzipCache(args []string) {
	flagVerbose := false
	flagOverwrite := false
	flagKeep := false
	for _, a := range args {
		a = strings.ToLower(a)
		if len(a) > 0 && a[0] == '-' {
			a = a[1:]
			if len(a) > 0 && a[0] == '-' {
				a = a[1:]
			} else {
				for _, aa := range a {
					switch aa {
					case 'v':
						flagVerbose = true
					case 'o':
						flagOverwrite = true
					case 'k':
						flagKeep = true
					default:
						fmt.Printf("Unknown option %q\n", aa)
						os.Exit(2)
					}
				}
				continue
			}
		}
		switch a {
		case "verbose", "v":
			flagVerbose = true
		case "overwrite", "o":
			flagOverwrite = true
		case "keep", "k":
			flagKeep = true
		default:
			fmt.Printf("Unknown option %q\n", a)
			os.Exit(2)
		}
	}
	cacheDir := filepath.Join(baseDir, "cache")
	fmt.Printf("Cache directory = %q\n", cacheDir)
	var hashBuf [64]byte
	err := walkCacheDir(cacheDir, func(hash string, _ int64) (_ error) {
		path := filepath.Join(cacheDir, hash[0:2], hash)
		target, ok := strings.CutSuffix(path, ".gz")
		if !ok {
			return
		}

		hashMethod, err := getHashMethod(len(hash))
		if err != nil {
			return
		}
		hw := hashMethod.New()

		if !flagOverwrite {
			if _, err := os.Stat(target); err == nil {
				return
			}
		}
		srcFd, err := os.Open(path)
		if err != nil {
			fmt.Printf("Error: could not open file %q: %v\n", path, err)
			return
		}
		defer srcFd.Close()
		if flagVerbose {
			fmt.Printf("decompressing %s\n", path)
		}
		tmpPath := target + ".tmp"
		var dstFd *os.File
		if dstFd, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
			fmt.Printf("Error: could not create %q: %v\n", tmpPath, err)
			return
		}
		defer dstFd.Close()
		r, err := gzip.NewReader(srcFd)
		if err != nil {
			fmt.Printf("Error: could not decompress %q: %v\n", path, err)
			return
		}
		if _, err = io.Copy(io.MultiWriter(dstFd, hw), r); err != nil {
			os.Remove(tmpPath)
			fmt.Printf("Error: could not decompress %q: %v\n", path, err)
			return
		}
		if hs := hex.EncodeToString(hw.Sum(hashBuf[:0])); hs != hash {
			os.Remove(tmpPath)
			fmt.Printf("Error: hash (%s) incorrect for %q. Got %s, want %s\n", hashMethod, path, hs, hash)
			return
		}
		os.Remove(target)
		if err = os.Rename(tmpPath, target); err != nil {
			os.Remove(tmpPath)
			fmt.Printf("Error: could not rename %q to %q\n", tmpPath, target)
			return
		}
		if !flagKeep {
			os.Remove(path)
		}
		return
	})
	if err != nil {
		fmt.Printf("Could not walk cache directory: %v", err)
		os.Exit(1)
	}
}
