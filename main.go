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
	"sync/atomic"
	"syscall"
	"time"

	"runtime/pprof"

	doh "github.com/libp2p/go-doh-resolver"

	"github.com/LiterMC/go-openbmclapi/config"
	"github.com/LiterMC/go-openbmclapi/cluster"
	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/lang"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	subcmds "github.com/LiterMC/go-openbmclapi/sub_commands"

	_ "github.com/LiterMC/go-openbmclapi/lang/en"
	_ "github.com/LiterMC/go-openbmclapi/lang/zh"
)

const ClusterServerURL = "https://openbmclapi.bangbang93.com"

var (
	KeepAliveInterval = time.Second * 59
)

var startTime = time.Now()

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
		// case "zip-cache":
		// 	cmdZipCache(os.Args[2:])
		// 	os.Exit(0)
		// case "unzip-cache":
		// 	cmdUnzipCache(os.Args[2:])
		// 	os.Exit(0)
		case "upload-webdav":
			subcmds.CmdUploadWebdav(os.Args[2:])
			os.Exit(0)
		default:
			fmt.Println("Unknown sub command:", subcmd)
			printHelp()
			os.Exit(0x7f)
		}
	}
}

func main() {
	if runtime.GOOS == "windows" {
		lang.SetLang("zh-cn")
	} else {
		lang.SetLang("en-us")
	}
	lang.ParseSystemLanguage()
	log.Debug("language:", lang.GetLang().Code())

	printShortLicense()
	parseArgs()

	defer log.RecordPanic()
	log.StartFlushLogFile()

	r := new(Runner)

	ctx, cancel := context.WithCancel(context.Background())

	if config, err := readAndRewriteConfig(); err != nil {
		log.Errorf("Config error: %s", err)
		os.Exit(1)
	} else {
		r.Config = config
	}
	if r.Config.Advanced.DebugLog {
		log.SetLevel(log.LevelDebug)
	} else {
		log.SetLevel(log.LevelInfo)
	}
	if r.Config.NoAccessLog {
		log.SetAccessLogSlots(-1)
	} else {
		log.SetAccessLogSlots(r.Config.AccessLogSlots)
	}

	r.Config.applyWebManifest(dsbManifest)

	log.TrInfof("program.starting", build.ClusterVersion, build.BuildVersion)

	if r.Config.Tunneler.Enable {
		r.StartTunneler()
	}
	r.InitServer()
	r.InitClusters(ctx)

	go func(ctx context.Context) {
		defer log.RecordPanic()

		if !r.cluster.Connect(ctx) {
			osExit(CodeClientOrServerError)
		}

		firstSyncDone := make(chan struct{}, 0)
		go func() {
			defer log.RecordPanic()
			defer close(firstSyncDone)
			r.InitSynchronizer(ctx)
		}()

		listener := r.CreateHTTPServerListener(ctx)
		go func(listener net.Listener) {
			defer listener.Close()
			if err := r.clusterSvr.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
				log.Error("Error when serving:", err)
				os.Exit(1)
			}
		}(listener)

		var publicHost string
		if len(r.publicHosts) == 0 {
			publicHost = config.PublicHost
		} else {
			publicHost = r.publicHosts[0]
		}
		if !config.Tunneler.Enable {
			strPort := strconv.Itoa((int)(r.getPublicPort()))
			log.TrInfof("info.server.public.at", net.JoinHostPort(publicHost, strPort), r.clusterSvr.Addr, r.getCertCount())
			if len(r.publicHosts) > 1 {
				log.TrInfof("info.server.alternative.hosts")
				for _, h := range r.publicHosts[1:] {
					log.Infof("\t- https://%s", net.JoinHostPort(h, strPort))
				}
			}
		}

		log.TrInfof("info.wait.first.sync")
		select {
		case <-firstSyncDone:
		case <-ctx.Done():
			return
		}

		r.EnableCluster(ctx)
	}(ctx)

	code := r.DoSignals(cancel)
	if code != 0 {
		log.TrErrorf("program.exited", code)
		log.TrErrorf("error.exit.please.read.faq")
		if runtime.GOOS == "windows" && !config.Advanced.DoNotOpenFAQOnWindows {
			// log.TrWarnf("warn.exit.detected.windows.open.browser")
			// cmd := exec.Command("cmd", "/C", "start", "https://cdn.crashmc.com/https://github.com/LiterMC/go-openbmclapi?tab=readme-ov-file#faq")
			// cmd.Start()
			time.Sleep(time.Hour)
		}
	}
	os.Exit(code)
}

type Runner struct {
	Config *config.Config

	clusters map[string]*Cluster
	server   *http.Server

	tlsConfig   *tls.Config
	listener    net.Listener
	publicHosts []string

	reloading    atomic.Bool
	updating     atomic.Bool
	tunnelCancel context.CancelFunc
}

func (r *Runner) getPublicPort() uint16 {
	if r.Config.PublicPort > 0 {
		return r.Config.PublicPort
	}
	return r.Config.Port
}

func (r *Runner) getCertCount() int {
	if r.tlsConfig == nil {
		return 0
	}
	return len(r.tlsConfig.Certificates)
}

func (r *Runner) DoSignals(cancel context.CancelFunc) int {
	signalCh := make(chan os.Signal, 1)
	log.Debugf("Receiving signals")
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(signalCh)

	var (
		forceStop context.CancelFunc
		exited    = make(chan struct{}, 0)
	)

	for {
		select {
		case <-exited:
			return 0
		case s := <-signalCh:
			switch s {
			case syscall.SIGQUIT:
				// avaliable commands see <https://pkg.go.dev/runtime/pprof#Profile>
				dumpCommand := "heap"
				dumpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("go-openbmclapi-dump-command.%d.in", os.Getpid()))
				log.Infof("Reading dump command file at %q", dumpFileName)
				var buf [128]byte
				if dumpFile, err := os.Open(dumpFileName); err != nil {
					log.Errorf("Cannot open dump command file: %v", err)
				} else if n, err := dumpFile.Read(buf[:]); err != nil {
					dumpFile.Close()
					log.Errorf("Cannot read dump command file: %v", err)
				} else {
					dumpFile.Truncate(0)
					dumpFile.Close()
					dumpCommand = (string)(bytes.TrimSpace(buf[:n]))
				}
				pcmd := pprof.Lookup(dumpCommand)
				if pcmd == nil {
					log.Errorf("No pprof command is named %q", dumpCommand)
					continue
				}
				name := fmt.Sprintf(time.Now().Format("dump-%s-20060102-150405.txt"), dumpCommand)
				log.Infof("Creating goroutine dump file at %s", name)
				if fd, err := os.Create(name); err != nil {
					log.Infof("Cannot create dump file: %v", err)
				} else {
					err := pcmd.WriteTo(fd, 1)
					fd.Close()
					if err != nil {
						log.Infof("Cannot write dump file: %v", err)
					} else {
						log.Info("Dump file created")
					}
				}
			case syscall.SIGHUP:
				go r.ReloadConfig()
			default:
				cancel()
				if forceStop == nil {
					ctx, cancel := context.WithCancel(context.Background())
					forceStop = cancel
					go func() {
						defer close(exited)
						r.StopServer(ctx)
					}()
				} else {
					log.Warn("signal:", s)
					log.Error("Second close signal received, forcely shutting down")
					forceStop()
				}
			}

		}
	}
	return 0
}

func (r *Runner) ReloadConfig() {
	if r.reloading.CompareAndSwap(false, true) {
		log.Error("Config is already reloading!")
		return
	}
	defer r.reloading.Store(false)

	config, err := readAndRewriteConfig()
	if err != nil {
		log.Errorf("Config error: %s", err)
	} else {
		r.Config = config
	}
}

func (r *Runner) StopServer(ctx context.Context) {
	r.tunnelCancel()
	shutCtx, cancelShut := context.WithTimeout(context.Background(), time.Second*15)
	defer cancelShut()
	log.TrWarnf("warn.server.closing")
	shutDone := make(chan struct{}, 0)
	go func() {
		defer close(shutDone)
		defer cancelShut()
		var wg sync.WaitGroup
		for _, cr := range r.clusters {
			go func() {
				defer wg.Done()
				cr.Disable(shutCtx)
			}()
		}
		wg.Wait()
		log.TrWarnf("warn.httpserver.closing")
		r.server.Shutdown(shutCtx)
	}()
	select {
	case <-shutDone:
	case <-ctx.Done():
		return
	}
	log.TrWarnf("warn.server.closed")
}

func (r *Runner) InitServer() {
	r.server = &http.Server{
		Addr:        fmt.Sprintf("%s:%d", d.Config.Host, d.Config.Port),
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 5 * time.Second,
		Handler:     r,
		ErrorLog:    log.ProxiedStdLog,
	}
}

func (r *Runner) InitClusters(ctx context.Context) {
	var (
		dialer *net.Dialer
		cache  = r.Config.Cache.newCache()
	)

	_ = doh.NewResolver // TODO: use doh resolver

	r.cluster = NewCluster(ctx,
		ClusterServerURL,
		baseDir,
		config.PublicHost, r.getPublicPort(),
		config.ClusterId, config.ClusterSecret,
		config.Byoc, dialer,
		config.Storages,
		cache,
	)
	if err := r.cluster.Init(ctx); err != nil {
		log.Errorf(Tr("error.init.failed"), err)
		os.Exit(1)
	}
}

func (r *Runner) UpdateFileRecords(files map[string]*cluster.StorageFileInfo, oldfileset map[string]int64) {
	if !r.hijacker.Enabled {
		return
	}
	if !r.updating.CompareAndSwap(false, true) {
		return
	}
	defer r.updating.Store(false)

	sem := limited.NewSemaphore(12)
	log.Info("Begin to update file records")
	for _, f := range files {
		if strings.HasPrefix(f.Path, "/openbmclapi/download/") {
			continue
		}
		if oldfileset[f.Hash] > 0 {
			continue
		}
		sem.Acquire()
		go func(rec database.FileRecord) {
			defer sem.Release()
			r.cluster.database.SetFileRecord(rec)
		}(database.FileRecord{
			Path: f.Path,
			Hash: f.Hash,
			Size: f.Size,
		})
	}
	sem.Wait()
	log.Info("All file records are updated")
}

func (r *Runner) InitSynchronizer(ctx context.Context) {
	fileMap := make(map[string]*StorageFileInfo)
	for _, cr := range r.clusters {
		log.Info(Tr("info.filelist.fetching"), cr.ID())
		if err := cr.GetFileList(ctx, fileMap, true); err != nil {
			log.Errorf(Tr("error.filelist.fetch.failed"), cr.ID(), err)
			if errors.Is(err, context.Canceled) {
				return
			}
		}
	}

	checkCount := -1
	heavyCheck := !config.Advanced.NoHeavyCheck
	heavyCheckInterval := config.Advanced.HeavyCheckInterval
	if heavyCheckInterval <= 0 {
		heavyCheck = false
	}

	if !config.Advanced.SkipFirstSync {
		if !r.cluster.SyncFiles(ctx, fileMap, false) {
			return
		}
		go r.UpdateFileRecords(fileMap, nil)

		if !config.Advanced.NoGC {
			go r.cluster.Gc()
		}
	} else if fl != nil {
		if err := r.cluster.SetFilesetByExists(ctx, fl); err != nil {
			return
		}
	}

	createInterval(ctx, func() {
		fileMap := make(map[string]*StorageFileInfo)
		for _, cr := range r.clusters {
			log.Info(Tr("info.filelist.fetching"), cr.ID())
			if err := cr.GetFileList(ctx, fileMap, false); err != nil {
				log.Errorf(Tr("error.filelist.fetch.failed"), cr.ID(), err)
				return
			}
		}
		if len(fileMap) == 0 {
			log.Infof("No file was updated since last check")
			return
		}

		checkCount = (checkCount + 1) % heavyCheckInterval
		oldfileset := r.cluster.CloneFileset()
		if r.cluster.SyncFiles(ctx, fl, heavyCheck && checkCount == 0) {
			go r.UpdateFileRecords(fl, oldfileset)
			if !config.Advanced.NoGC && !config.OnlyGcWhenStart {
				go r.cluster.Gc()
			}
		}
	}, (time.Duration)(config.SyncInterval)*time.Minute)
}

func (r *Runner) CreateHTTPServerListener(ctx context.Context) (listener net.Listener) {
	listener, err := net.Listen("tcp", r.Addr)
	if err != nil {
		log.Errorf(Tr("error.address.listen.failed"), r.Addr, err)
		osExit(CodeEnvironmentError)
	}
	if r.Config.ServeLimit.Enable {
		limted := limited.NewLimitedListener(listener, config.ServeLimit.MaxConn, 0, config.ServeLimit.UploadRate*1024)
		limted.SetMinWriteRate(1024)
		listener = limted
	}

	tlsConfig := r.GenerateTLSConfig(ctx)
	r.publicHosts = make([]string, 0, 2)
	if tlsConfig != nil {
		for _, cert := range tlsConfig.Certificates {
			if h, err := parseCertCommonName(cert.Certificate[0]); err == nil {
				r.publicHosts = append(r.publicHosts, strings.ToLower(h))
			}
		}
		listener = utils.NewHttpTLSListener(listener, tlsConfig, r.publicHosts, r.getPublicPort())
	}
	r.listener = listener
	return
}

func (r *Runner) GenerateTLSConfig(ctx context.Context) (tlsConfig *tls.Config) {
	if config.UseCert {
		if len(config.Certificates) == 0 {
			log.Error(Tr("error.cert.not.set"))
			os.Exit(1)
		}
		tlsConfig = new(tls.Config)
		tlsConfig.Certificates = make([]tls.Certificate, len(config.Certificates))
		for i, c := range config.Certificates {
			var err error
			tlsConfig.Certificates[i], err = tls.LoadX509KeyPair(c.Cert, c.Key)
			if err != nil {
				log.Errorf(Tr("error.cert.parse.failed"), i, err)
				os.Exit(1)
			}
		}
	}
	if !config.Byoc {
		for _, cr := range r.clusters {
			log.Info(Tr("info.cert.requesting"), cr.ID())
			tctx, cancel := context.WithTimeout(ctx, time.Minute*10)
			pair, err := cr.RequestCert(tctx)
			cancel()
			if err != nil {
				log.Errorf(Tr("error.cert.request.failed"), err)
				os.Exit(2)
			}
			if tlsConfig == nil {
				tlsConfig = new(tls.Config)
			}
			var cert tls.Certificate
			cert, err = tls.X509KeyPair(([]byte)(pair.Cert), ([]byte)(pair.Key))
			if err != nil {
				log.Errorf(Tr("error.cert.requested.parse.failed"), err)
				os.Exit(2)
			}
			tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
			certHost, _ := parseCertCommonName(cert.Certificate[0])
			log.Infof(Tr("info.cert.requested"), certHost)
		}
	}
	r.tlsConfig = tlsConfig
	return
}

func (r *Runner) EnableCluster(ctx context.Context) {
	if config.Advanced.WaitBeforeEnable > 0 {
		select {
		case <-time.After(time.Second * (time.Duration)(config.Advanced.WaitBeforeEnable)):
		case <-ctx.Done():
			return
		}
	}

	if config.Tunneler.Enable {
		r.enableClusterByTunnel(ctx)
	} else {
		if err := r.cluster.Enable(ctx); err != nil {
			log.Errorf(Tr("error.cluster.enable.failed"), err)
			if ctx.Err() != nil {
				return
			}
			osExit(CodeServerOrEnvionmentError)
		}
	}
}

func (r *Runner) StartTunneler() {
	ctx, cancel := context.WithCancel(context.Background())
	r.tunnelCancel = cancel
	go func() {
		dur := time.Second
		for {
			start := time.Now()
			r.RunTunneler(ctx)
			used := time.Since(start)
			// If the program runs no longer than 30s, then it fails too fast.
			if used < time.Second*30 {
				dur = min(dur*2, time.Minute*10)
			} else {
				dur = time.Second
			}
			select {
			case <-time.After(dur):
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (r *Runner) RunTunneler(ctx context.Context) {
	cmd := exec.CommandContext(ctx, config.Tunneler.TunnelProg)
	log.Infof(Tr("info.tunnel.running"), cmd.String())
	var (
		cmdOut, cmdErr io.ReadCloser
		err            error
	)
	cmd.Env = append(os.Environ(),
		"CLUSTER_PORT="+strconv.Itoa((int)(r.Config.Port)))
	if cmdOut, err = cmd.StdoutPipe(); err != nil {
		log.Errorf(Tr("error.tunnel.command.prepare.failed"), err)
		os.Exit(1)
	}
	if cmdErr, err = cmd.StderrPipe(); err != nil {
		log.Errorf(Tr("error.tunnel.command.prepare.failed"), err)
		os.Exit(1)
	}
	if err = cmd.Start(); err != nil {
		log.Errorf(Tr("error.tunnel.command.prepare.failed"), err)
		os.Exit(1)
	}
	type addrOut struct {
		host string
		port uint16
	}
	detectedCh := make(chan addrOut, 1)
	onLog := func(line []byte) {
		res := config.Tunneler.outputRegex.FindSubmatch(line)
		if res == nil {
			return
		}
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
	go func() {
		defer cmdOut.Close()
		defer cmd.Process.Kill()
		sc := bufio.NewScanner(cmdOut)
		for sc.Scan() {
			log.Info("[tunneler/stdout]:", sc.Text())
			onLog(sc.Bytes())
		}
	}()
	go func() {
		defer cmdErr.Close()
		defer cmd.Process.Kill()
		sc := bufio.NewScanner(cmdErr)
		for sc.Scan() {
			log.Info("[tunneler/stderr]:", sc.Text())
			onLog(sc.Bytes())
		}
	}()
	go func() {
		defer log.RecordPanic()
		defer cmd.Process.Kill()
		for {
			select {
			case addr := <-detectedCh:
				log.Infof(Tr("info.tunnel.detected"), addr.host, addr.port)
				r.cluster.publicPort = addr.port
				if !r.cluster.byoc {
					r.cluster.host = addr.host
				}
				strPort := strconv.Itoa((int)(r.getPublicPort()))
				if spp, ok := r.listener.(interface{ SetPublicPort(port string) }); ok {
					spp.SetPublicPort(strPort)
				}
				log.Infof(Tr("info.server.public.at"), net.JoinHostPort(addr.host, strPort), r.clusterSvr.Addr, r.getCertCount())
				if len(r.publicHosts) > 1 {
					log.Info(Tr("info.server.alternative.hosts"))
					for _, h := range r.publicHosts[1:] {
						log.Infof("\t- https://%s", net.JoinHostPort(h, strPort))
					}
				}
				if !r.cluster.Enabled() {
					shutCtx, cancel := context.WithTimeout(ctx, time.Minute)
					r.cluster.Disable(shutCtx)
					cancel()
				}
				if err := r.cluster.Enable(ctx); err != nil {
					log.Errorf(Tr("error.cluster.enable.failed"), err)
					if ctx.Err() != nil {
						return
					}
					os.Exit(2)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	if _, err := cmd.Process.Wait(); err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Errorf("Tunnel program exited: %v", err)
	}
}
