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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	SyncFileInterval  = time.Minute * 10
	KeepAliveInterval = time.Second * 59
)

var startTime = time.Now()

var config Config

const baseDir = "."

func main() {
	printShortLicense()
	if len(os.Args) > 1 {
		subcmd := strings.ToLower(os.Args[1])
		switch subcmd {
		case "license":
			printLongLicense()
			os.Exit(0)
		default:
			fmt.Println("Unknown sub command:", subcmd)
			os.Exit(-1)
		}
	}

	defer func() {
		if err := recover(); err != nil {
			logError("Panic error:", err)
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
		time.Sleep(time.Second * 100000)
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
			go func(ctx context.Context, item *OSSItem) {
				ticker := time.NewTicker(time.Minute * 5)
				defer ticker.Stop()

				online := true
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
							if online {
								online = false
								if aliveCount.Add(-1) == 0 {
									logError("All oss mirror failed, exit.")
									os.Exit(2)
								}
							}
							continue
						}
						if !online {
							online = true
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
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second,
		Handler:     cluster.GetHandler(),
	}

	go func(ctx context.Context) {
		listener, err := net.Listen("tcp", clusterSvr.Addr)
		if err != nil {
			logErrorf("Cannot listen on %s: %v", clusterSvr.Addr, err)
			os.Exit(1)
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
		cluster.SyncFiles(ctx, fl)

		createInterval(ctx, func() {
			logInfof("Fetching file list")
			fl, err := cluster.GetFileList(ctx)
			if err != nil {
				logError("Cannot query cluster file list:", err)
				return
			}
			cluster.SyncFiles(ctx, fl)
		}, SyncFileInterval)

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

func createOssMirrorDir(item *OSSItem) {
	logInfof("Creating %s", item.FolderPath)
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
	if supportRange = res.StatusCode == http.StatusPartialContent; supportRange {
		logDebug("OSS support range header!")
		targetSize--
	} else if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("OSS check request failed %q: %d %s", target, res.StatusCode, res.Status)
	}
	logDebug("reading OSS response")
	start := time.Now()
	n, err := io.Copy(io.Discard, res.Body)
	if err != nil {
		return false, fmt.Errorf("OSS check request failed %q: %w", target, err)
	}
	used := time.Since(start)
	if n != targetSize {
		return false, fmt.Errorf("OSS check request failed %q: expected %dMB, but got %d bytes", target, size, n)
	}
	logInfof("Check finished for %q, used %v, %s/s; supportRange=%v", target, used, bytesToUnit((float64)(n)/used.Seconds()), supportRange)
	return
}
