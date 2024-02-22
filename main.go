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
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"runtime/pprof"
)

const ClusterServerURL = "https://openbmclapi.bangbang93.com"

var (
	KeepAliveInterval = time.Second * 59
)

var startTime = time.Now()

var config Config = defaultConfig

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
		case "version", "--version":
			fmt.Printf("Go-OpenBmclApi v%s (%s)\n", ClusterVersion, BuildVersion)
			os.Exit(0)
		case "help", "--help":
			printHelp()
			os.Exit(0)
		case "zip-cache":
			cmdZipCache(os.Args[2:])
			os.Exit(0)
		case "unzip-cache":
			cmdUnzipCache(os.Args[2:])
			os.Exit(0)
		case "upload-webdav":
			cmdUploadWebdav(os.Args[2:])
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

	config.applyWebManifest(dsbManifest)

	var (
		dialer *net.Dialer
	)

	logInfof("Starting Go-OpenBmclApi v%s (%s)", ClusterVersion, BuildVersion)

	cache := config.Cache.newCache()

	publicPort := config.PublicPort
	if publicPort == 0 {
		publicPort = config.Port
	}
	cluster := NewCluster(ctx,
		ClusterServerURL,
		baseDir,
		config.PublicHost, publicPort,
		config.ClusterId, config.ClusterSecret,
		config.Byoc, dialer,
		config.Storages,
		cache,
	)
	if err := cluster.Init(ctx); err != nil {
		logError("Cannot init cluster:", err)
		os.Exit(1)
	}

	if !cluster.Connect(ctx) {
		os.Exit(1)
	}

	logDebugf("Receiving signals")
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	clusterSvr := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", "0.0.0.0", config.Port),
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 5 * time.Second,
		Handler:     cluster.GetHandler(),
		ErrorLog:    NullLogger, // for ignore TLS handshake error
	}

	go func(ctx context.Context) {
		listener, err := net.Listen("tcp", clusterSvr.Addr)
		if err != nil {
			logErrorf("Cannot listen on %s: %v", clusterSvr.Addr, err)
			os.Exit(1)
		}
		if config.ServeLimit.Enable {
			limited := NewLimitedListener(listener, config.ServeLimit.MaxConn, 0, config.ServeLimit.UploadRate*1024)
			limited.SetMinWriteRate(1024)
			listener = limited
		}

		if !config.Byoc {
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
			logInfof("Server public at https://%s:%d (%s)", publicHost, publicPort, clusterSvr.Addr)
		} else {
			go func() {
				defer listener.Close()
				if err = clusterSvr.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
					logError("Error on server:", err)
					os.Exit(1)
				}
			}()
			logInfof("Server public at https://%s:%d (%s)", config.PublicHost, publicPort, clusterSvr.Addr)
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
		checkCount := -1
		heavyCheck := !config.Advanced.NoHeavyCheck

		if !config.Advanced.SkipFirstSync {
			cluster.SyncFiles(ctx, fl, false)
			if ctx.Err() != nil {
				return
			}
		} else {
			fileset := make(map[string]int64, len(fl))
			for _, f := range fl {
				fileset[f.Hash] = f.Size
			}
			cluster.mux.Lock()
			cluster.fileset = fileset
			cluster.mux.Unlock()
		}
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

SELECT_SIGNAL:
	select {
	case s := <-signalCh:
		if s == syscall.SIGQUIT {
			name := time.Now().Format("dump-20060102-150405.txt")
			logInfof("Creating goroutine dump file at %s", name)
			if fd, err := os.Create(name); err != nil {
				logInfof("Cannot create dump file: %v", err)
			} else {
				err := pprof.Lookup("goroutine").WriteTo(fd, 1)
				fd.Close()
				if err != nil {
					logInfof("Cannot write dump file: %v", err)
				} else {
					logInfo("Dump file created")
				}
			}
			goto SELECT_SIGNAL
		}
		cancel()
		shutCtx, cancelShut := context.WithTimeout(context.Background(), 20*time.Second)
		logWarn("Closing server ...")
		shutExit := make(chan struct{}, 0)
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
