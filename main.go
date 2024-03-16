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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"runtime/pprof"

	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/lang"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"

	_ "github.com/LiterMC/go-openbmclapi/lang/en"
	_ "github.com/LiterMC/go-openbmclapi/lang/zh"
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
			fmt.Printf("Go-OpenBmclApi v%s (%s)\n", build.ClusterVersion, build.BuildVersion)
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

var exitCh = make(chan int, 1)

func osExit(n int) {
	select {
	case exitCh <- n:
	default:
	}
	runtime.Goexit()
}

func main() {
	if runtime.GOOS == "windows" {
		lang.SetLang("zh-cn")
	} else {
		lang.SetLang("en-us")
	}
	lang.ParseSystemLanguage()
	log.Info("language:", lang.GetLang().Code())

	printShortLicense()
	parseArgs()

	exitCode := -1
	defer func() {
		code := exitCode
		if code < 0 {
			select {
			case code = <-exitCh:
			default:
				code = 0
			}
		}
		if code != 0 {
			log.Errorf(Tr("program.exited"), exitCode)
			log.Error(Tr("error.exit.please.read.faq"))
			if runtime.GOOS == "windows" && !config.Advanced.DoNotOpenFAQOnWindows {
				log.Warn(Tr("warn.exit.detected.windows.open.browser"))
				cmd := exec.Command("cmd", "/C", "start", "https://cdn.crashmc.com/https://github.com/LiterMC/go-openbmclapi?tab=readme-ov-file#faq")
				cmd.Start()
				time.Sleep(time.Hour)
			}
		}
		os.Exit(code)
	}()
	defer log.RecordPanic()
	log.StartFlushLogFile()

	// override the osExit inside main function
	osExit := func(n int) {
		exitCode = n
		runtime.Goexit()
	}

	bgctx := context.Background()

	signalCh := make(chan os.Signal, 1)

	var (
		dumpCmdFile = filepath.Join(os.TempDir(), fmt.Sprintf("go-openbmclapi-dump-command.%d.in", os.Getpid()))
		dumpCmdFd   *os.File
	)
	if config.Advanced.DebugLog {
		var err error
		if dumpCmdFd, err = os.OpenFile(dumpCmdFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0660); err != nil {
			log.Errorf("Cannot create %q: %v", dumpCmdFile, err)
		} else {
			defer os.Remove(dumpCmdFile)
			defer dumpCmdFd.Close()
			log.Infof("Dump command file %s has created", dumpCmdFile)
		}
	}

START:
	signal.Stop(signalCh)

	ctx, cancel := context.WithCancel(bgctx)

	config = readConfig()
	if config.Advanced.DebugLog {
		log.SetLevel(log.LevelDebug)
	} else {
		log.SetLevel(log.LevelInfo)
	}
	if config.NoAccessLog {
		log.SetAccessLogSlots(-1)
	} else {
		log.SetAccessLogSlots(config.AccessLogSlots)
	}

	config.applyWebManifest(dsbManifest)

	var (
		dialer *net.Dialer
	)

	log.Infof(Tr("program.starting"), build.ClusterVersion, build.BuildVersion)

	if config.ClusterId == defaultConfig.ClusterId || config.ClusterSecret == defaultConfig.ClusterSecret {
		log.Error(Tr("error.set.cluster.id"))
		osExit(1)
	}

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
		log.Errorf(Tr("error.init.failed"), err)
		osExit(1)
	}

	if !cluster.Connect(ctx) {
		osExit(1)
	}

	log.Debugf("Receiving signals")
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	clusterSvr := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", "0.0.0.0", config.Port),
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 5 * time.Second,
		Handler:     cluster.GetHandler(),
		ErrorLog:    log.ProxiedStdLog,
	}

	firstSyncDone := make(chan struct{}, 0)

	go func(ctx context.Context) {
		defer log.RecordPanic()
		defer close(firstSyncDone)
		log.Info(Tr("info.filelist.fetching"))
		fl, err := cluster.GetFileList(ctx)
		if err != nil {
			log.Errorf(Tr("error.filelist.fetch.failed"), err)
			if errors.Is(err, context.Canceled) {
				return
			}
			if !config.Advanced.SkipFirstSync {
				osExit(1)
			}
		}
		checkCount := -1
		heavyCheck := !config.Advanced.NoHeavyCheck
		heavyCheckInterval := config.Advanced.HeavyCheckInterval
		if heavyCheckInterval <= 0 {
			heavyCheck = false
		}

		if !config.Advanced.SkipFirstSync {
			cluster.SyncFiles(ctx, fl, false)

			if !config.Advanced.NoGC {
				go cluster.Gc()
			}

			if ctx.Err() != nil {
				return
			}
		} else if fl != nil {
			if err := cluster.SetFilesetByExists(ctx, fl); err != nil {
				return
			}
		}
		createInterval(ctx, func() {
			log.Info(Tr("info.fetch.filelist"))
			fl, err := cluster.GetFileList(ctx)
			if err != nil {
				log.Errorf(Tr("error.cannot.fetch.filelist"), err)
				return
			}
			checkCount = (checkCount + 1) % heavyCheckInterval
			cluster.SyncFiles(ctx, fl, heavyCheck && checkCount == 0)
			if !config.Advanced.NoGC && !config.OnlyGcWhenStart {
				go cluster.Gc()
			}
		}, (time.Duration)(config.SyncInterval)*time.Minute)
	}(ctx)

	go func(ctx context.Context) {
		defer log.RecordPanic()
		listener, err := net.Listen("tcp", clusterSvr.Addr)
		if err != nil {
			log.Errorf(Tr("error.address.listen.failed"), clusterSvr.Addr, err)
			osExit(1)
		}
		if config.ServeLimit.Enable {
			limted := limited.NewLimitedListener(listener, config.ServeLimit.MaxConn, 0, config.ServeLimit.UploadRate*1024)
			limted.SetMinWriteRate(1024)
			listener = limted
		}

		var tlsConfig *tls.Config
		var publicHosts []string
		if config.UseCert {
			if len(config.Certificates) == 0 {
				log.Error(Tr("error.cert.not.set"))
				osExit(1)
			}
			tlsConfig = new(tls.Config)
			tlsConfig.Certificates = make([]tls.Certificate, len(config.Certificates))
			for i, c := range config.Certificates {
				var err error
				tlsConfig.Certificates[i], err = tls.LoadX509KeyPair(c.Cert, c.Key)
				if err != nil {
					log.Errorf(Tr("error.cert.parse.failed"), i, err)
					osExit(1)
				}
			}
		}
		if !config.Byoc {
			log.Info(Tr("info.cert.requesting"))
			tctx, cancel := context.WithTimeout(ctx, time.Minute*10)
			pair, err := cluster.RequestCert(tctx)
			cancel()
			if err != nil {
				log.Errorf(Tr("error.cert.request.failed"), err)
				osExit(1)
			}
			if tlsConfig == nil {
				tlsConfig = new(tls.Config)
			}
			var cert tls.Certificate
			cert, err = tls.X509KeyPair(([]byte)(pair.Cert), ([]byte)(pair.Key))
			if err != nil {
				log.Errorf(Tr("error.cert.requested.parse.failed"), err)
				osExit(1)
			}
			tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
			certHost, _ := parseCertCommonName(cert.Certificate[0])
			log.Infof(Tr("info.cert.requested"), certHost)
		}
		certCount := 0
		publicHost := ""
		if tlsConfig != nil {
			certCount = len(tlsConfig.Certificates)
			for _, cert := range tlsConfig.Certificates {
				if h, err := parseCertCommonName(cert.Certificate[0]); err == nil {
					if publicHost == "" {
						publicHost = h
					}
					publicHosts = append(publicHosts, strings.ToLower(h))
				}
			}
			cluster.publicHosts = publicHosts
			listener = newHttpTLSListener(listener, tlsConfig, publicHosts, publicPort)
		}
		go func(listener net.Listener) {
			defer listener.Close()
			if err = clusterSvr.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
				log.Error("Error on server:", err)
				osExit(1)
			}
		}(listener)
		if publicHost == "" {
			publicHost = config.PublicHost
		}
		log.Infof(Tr("info.server.public.at"), publicHost, publicPort, clusterSvr.Addr, certCount)
		if len(publicHosts) > 1 {
			log.Info(Tr("info.server.alternative.hosts"))
			for _, h := range publicHosts[1:] {
				log.Infof("\t- https://%s:%d", h, publicPort)
			}
		}

		log.Info(Tr("info.wait.first.sync"))
		select {
		case <-firstSyncDone:
		case <-ctx.Done():
			return
		}

		if config.Advanced.WaitBeforeEnable > 0 {
			select {
			case <-time.After(time.Second * (time.Duration)(config.Advanced.WaitBeforeEnable)):
			case <-ctx.Done():
				return
			}
		}

		if config.Tunneler.Enable {
			cmd := exec.CommandContext(ctx, config.Tunneler.TunnelProg)
			log.Infof(Tr("info.tunnel.running"), cmd.String())
			var (
				cmdOut, cmdErr io.ReadCloser
				err            error
			)
			if cmdOut, err = cmd.StdoutPipe(); err != nil {
				log.Errorf(Tr("error.tunnel.command.prepare.failed"), err)
				osExit(9)
			}
			if cmdErr, err = cmd.StderrPipe(); err != nil {
				log.Errorf(Tr("error.tunnel.command.prepare.failed"), err)
				osExit(9)
			}
			if err = cmd.Start(); err != nil {
				log.Errorf(Tr("error.tunnel.command.prepare.failed"), err)
				osExit(1)
			}
			go func() {
				defer cmdErr.Close()
				defer cmd.Process.Kill()
				sc := bufio.NewScanner(cmdErr)
				for sc.Scan() {
					log.Info("[tunneler/stderr]:", sc.Text())
				}
			}()
			type addrOut struct {
				host string
				port uint16
			}
			detectedCh := make(chan addrOut, 0)
			go func() {
				defer cmdOut.Close()
				defer cmd.Process.Kill()
				sc := bufio.NewScanner(cmdOut)
				for sc.Scan() {
					log.Info("[tunneler/stdout]:", sc.Text())
					if res := config.Tunneler.outputRegex.FindSubmatch(sc.Bytes()); res != nil {
						tunnelHost, tunnelPort := res[config.Tunneler.hostOut], res[config.Tunneler.portOut]
						if len(tunnelHost) > 0 && tunnelHost[0] == '[' && tunnelHost[len(tunnelHost)-1] == ']' { // a IPv6 with port [<ipv6>]:<port>
							tunnelHost = tunnelHost[1 : len(tunnelHost)-1]
						}
						port, err := strconv.Atoi((string)(tunnelPort))
						if err != nil {
							log.Panic(err)
						}
						select {
						case detectedCh <- addrOut{
							host: (string)(tunnelHost),
							port: (uint16)(port),
						}:
						default:
						}
					}
				}
			}()
			go func() {
				defer cmd.Process.Kill()
				for {
					select {
					case addr := <-detectedCh:
						cluster.host = addr.host
						cluster.publicPort = addr.port
						log.Infof(Tr("info.tunnel.detected"), cluster.host, cluster.publicPort)
						if !cluster.Enabled() {
							shutCtx, cancel := context.WithTimeout(ctx, time.Minute)
							cluster.Disable(shutCtx)
							cancel()
						}
						if err := cluster.Enable(ctx); err != nil {
							log.Errorf(Tr("error.cluster.enable.failed"), err)
							osExit(1)
						}
					case <-ctx.Done():
						return
					}
				}
			}()
			// TODO: maybe restart the tunnel program?
		} else {
			if err := cluster.Enable(ctx); err != nil {
				log.Errorf(Tr("error.cluster.enable.failed"), err)
				osExit(1)
			}
		}
	}(ctx)

SELECT_SIGNAL:
	select {
	case code := <-exitCh:
		exitCode = code
		return
	case s := <-signalCh:
		if s == syscall.SIGQUIT {
			// avaliable commands see <https://pkg.go.dev/runtime/pprof#Profile>
			dumpCommand := "heap"
			if dumpCmdFd != nil {
				var buf [256]byte
				if _, err := dumpCmdFd.Seek(0, io.SeekStart); err != nil {
					log.Errorf("Cannot read dump command file: %v", err)
				} else if n, err := dumpCmdFd.Read(buf[:]); err != nil {
					log.Errorf("Cannot read dump command file: %v", err)
				} else {
					dumpCommand = (string)(bytes.TrimSpace(buf[:n]))
				}
				dumpCmdFd.Truncate(0)
			}
			name := fmt.Sprintf(time.Now().Format("dump-%s-20060102-150405.txt"), dumpCommand)
			log.Infof("Creating goroutine dump file at %s", name)
			if fd, err := os.Create(name); err != nil {
				log.Infof("Cannot create dump file: %v", err)
			} else {
				err := pprof.Lookup(dumpCommand).WriteTo(fd, 1)
				fd.Close()
				if err != nil {
					log.Infof("Cannot write dump file: %v", err)
				} else {
					log.Info("Dump file created")
				}
			}
			goto SELECT_SIGNAL
		}

		cancel()
		shutCtx, cancelShut := context.WithTimeout(context.Background(), time.Minute)
		log.Warn(Tr("warn.server.closing"))
		shutExit := make(chan struct{}, 0)
		go func() {
			defer close(shutExit)
			defer cancelShut()
			cluster.Disable(shutCtx)
			log.Warn(Tr("warn.httpserver.closing"))
			clusterSvr.Shutdown(shutCtx)
		}()
		select {
		case <-shutExit:
		case s := <-signalCh:
			log.Warn("signal:", s)
			log.Error("Second close signal received, exit")
			return
		}
		log.Warn(Tr("warn.server.closed"))
		if s == syscall.SIGHUP {
			log.Info("Restarting server ...")
			goto START
		}
	}
}

func Tr(name string) string {
	return lang.Tr(name)
}
