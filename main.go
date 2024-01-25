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
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	KeepAliveInterval = time.Second * 59
)

var startTime = time.Now()

var config Config

const baseDir = "."

func parseArgs() {
	if len(os.Args) > 1 {
		subcmd := strings.ToLower(os.Args[1])
		switch subcmd {
		case "main", "serve":
			break
		case "license":
			printLongLicense()
			os.Exit(0)
		case "zip-cache":
			flagVerbose := false
			flagAll := false
			flagOverwrite := false
			flagKeep := false
			for _, a := range os.Args[2:] {
				switch strings.ToLower(a) {
				case "verbose", "v":
					flagVerbose = true
				case "all", "a":
					flagAll = true
				case "overwrite", "o":
					flagOverwrite = true
				case "keep", "k":
					flagKeep = true
				}
			}
			cacheDir := filepath.Join(baseDir, "cache")
			fmt.Printf("Cache directory = %q\n", cacheDir)
			err := walkCacheDir(cacheDir, func(path string) (_ error) {
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
			os.Exit(0)
		case "unzip-cache":
			flagVerbose := false
			flagOverwrite := false
			flagKeep := false
			for _, a := range os.Args[2:] {
				switch strings.ToLower(a) {
				case "verbose", "v":
					flagVerbose = true
				case "overwrite", "o":
					flagOverwrite = true
				case "keep", "k":
					flagKeep = true
				}
			}
			cacheDir := filepath.Join(baseDir, "cache")
			fmt.Printf("Cache directory = %q\n", cacheDir)
			var hashBuf [64]byte
			err := walkCacheDir(cacheDir, func(path string) (_ error) {
				target, ok := strings.CutSuffix(path, ".gz")
				if !ok {
					return
				}

				hash := filepath.Base(target)
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
			os.Exit(0)
		default:
			fmt.Println("Unknown sub command:", subcmd)
			printHelp()
			os.Exit(0x7f)
		}
	}
}

func main() {
	printShortLicense()
	parseArgs()

	defer func() {
		if err := recover(); err != nil {
			logError("Panic:", err)
			panic(err)
		}
	}()

	startFlushLogFile()

	bgctx := context.Background()

	signalCh := make(chan os.Signal, 1)

START:
	signal.Stop(signalCh)

	ctx, cancel := context.WithCancel(bgctx)

	config = readConfig()

	httpcli := &http.Client{
		Timeout: 30 * time.Second,
	}

	var (
		dialer   *net.Dialer
		hjproxy  *HjProxy
		hjServer *http.Server
	)
	if config.Hijack.Enable {
		dialer = getDialerWithDNS(config.Hijack.AntiHijackDNS)
		hjproxy = NewHjProxy(dialer, config.Hijack.Path)
		hjServer = &http.Server{
			Addr:    fmt.Sprintf("%s:%d", config.Hijack.ServerHost, config.Hijack.ServerPort),
			Handler: hjproxy,
		}
		go func() {
			logInfof("Hijack server start at %q", hjServer.Addr)
			err := hjServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logError("Error on server:", err)
				os.Exit(1)
			}
		}()
	}

	var ossList []*OSSItem
	if config.Oss.Enable {
		ossList = config.Oss.List
	}

	logInfof("Starting Go-OpenBmclApi v%s (%s)", ClusterVersion, BuildVersion)
	cluster, err := NewCluster(ctx, baseDir,
		config.PublicHost, config.PublicPort,
		config.ClusterId, config.ClusterSecret,
		config.Nohttps, dialer,
		ossList,
	)
	if err != nil {
		logError("Cannot init cluster:", err)
		os.Exit(1)
	}

	if config.Oss.Enable {
		var aliveCount atomic.Int32
		for _, item := range config.Oss.List {
			createOssMirrorDir(item)
			supportRange, err := checkOSS(ctx, httpcli, item, 10)
			if err != nil {
				logError(err)
				continue
			}
			aliveCount.Add(1)
			item.supportRange = supportRange
			item.working.Store(true)
			go func(ctx context.Context, item *OSSItem) {
				ticker := time.NewTicker(time.Minute * 5)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						supportRange, err := checkOSS(ctx, httpcli, item, 1)
						if err != nil {
							if errors.Is(err, context.Canceled) {
								return
							}
							logError(err)
							if item.working.CompareAndSwap(true, false) {
								if aliveCount.Add(-1) == 0 {
									logError("All oss mirror failed, exit.")
									os.Exit(2)
								}
							}
							continue
						}
						if item.working.CompareAndSwap(false, true) {
							aliveCount.Add(1)
						}
						_ = supportRange
					}
				}
			}(ctx, item)
		}
		if aliveCount.Load() == 0 {
			logError("All oss mirror failed, exit.")
			os.Exit(2)
		}
	}

	logDebugf("Receiving signals")
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	if !cluster.Connect(ctx) {
		os.Exit(1)
	}

	clusterSvr := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", "0.0.0.0", config.Port),
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 5 * time.Second,
		Handler:     cluster.GetHandler(),
	}

	go func(ctx context.Context) {
		listener, err := net.Listen("tcp", clusterSvr.Addr)
		if err != nil {
			logErrorf("Cannot listen on %s: %v", clusterSvr.Addr, err)
			os.Exit(1)
		}
		if config.ServeLimit.Enable {
			limited := NewLimitedListener(listener, config.ServeLimit.MaxConn)
			limited.SetWriteRate(config.ServeLimit.UploadRate * 1024)
			limited.SetMinWriteRate(1024)
			listener = limited
		}

		if !config.Nohttps {
			tctx, cancel := context.WithTimeout(ctx, time.Minute*10)
			pair, err := cluster.RequestCert(tctx)
			cancel()
			if err != nil {
				logError("Error when requesting cert key pair:", err)
				os.Exit(1)
			}
			publicHost, _ := parseCertCommonName(([]byte)(pair.Cert))
			certFile, keyFile, err := pair.SaveAsFile()
			if err != nil {
				logError("Error when saving cert key pair:", err)
				os.Exit(1)
			}
			go func() {
				defer listener.Close()
				if err = clusterSvr.ServeTLS(listener, certFile, keyFile); !errors.Is(err, http.ErrServerClosed) {
					logError("Error on server:", err)
					os.Exit(1)
				}
			}()
			if publicHost == "" {
				publicHost = config.PublicHost
			}
			logInfof("Server public at https://%s:%d (%s)", publicHost, config.PublicPort, clusterSvr.Addr)
		} else {
			go func() {
				defer listener.Close()
				if err = clusterSvr.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
					logError("Error on server:", err)
					os.Exit(1)
				}
			}()
			logInfof("Server public at http://%s:%d (%s)", config.PublicHost, config.PublicPort, clusterSvr.Addr)
		}

		logInfof("Fetching file list")
		fl, err := cluster.GetFileList(ctx)
		if err != nil {
			logError("Cannot query cluster file list:", err)
			if errors.Is(err, context.Canceled) {
				return
			}
			os.Exit(1)
		}
		cluster.SyncFiles(ctx, fl, false)

		checkCount := 0
		heavyCheck := !config.NoHeavyCheck
		createInterval(ctx, func() {
			logInfof("Fetching file list")
			fl, err := cluster.GetFileList(ctx)
			if err != nil {
				logError("Cannot query cluster file list:", err)
				return
			}
			checkCount = (checkCount + 1) % 10
			cluster.SyncFiles(ctx, fl, heavyCheck && checkCount == 0)
		}, (time.Duration)(config.SyncInterval)*time.Minute)

		if err := cluster.Enable(ctx); err != nil {
			logError("Cannot enable cluster:", err)
			os.Exit(1)
		}
	}(ctx)

	select {
	case s := <-signalCh:
		cancel()
		shutCtx, cancelShut := context.WithTimeout(context.Background(), 20*time.Second)
		logWarn("Closing server ...")
		shutExit := make(chan struct{}, 0)
		if hjServer != nil {
			go hjServer.Shutdown(shutCtx)
		}
		go func() {
			defer close(shutExit)
			defer cancelShut()
			cluster.Disable(shutCtx)
			logInfo("Cluster disabled, closing http server")
			clusterSvr.Shutdown(shutCtx)
		}()
		select {
		case <-shutExit:
		case s := <-signalCh:
			logWarn("signal:", s)
			logError("Second close signal received, exit")
			return
		}
		logWarn("Server closed.")
		if s == syscall.SIGHUP {
			logInfo("Restarting server ...")
			goto START
		}
	}
}
