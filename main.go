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
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/LiterMC/go-openbmclapi/cluster"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/lang"
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
const dataDir = "data"

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

	ctx, cancel := context.WithCancel(context.Background())

	config, err := readAndRewriteConfig()
	if err != nil {
		log.Errorf("Config error: %s", err)
		os.Exit(1)
	}

	r := NewRunner(cfg)
	r.SetupLogger(ctx)

	log.TrInfof("program.starting", build.ClusterVersion, build.BuildVersion)

	if r.Config.Tunneler.Enable {
		r.StartTunneler()
	}
	r.InitServer()
	r.StartServer(ctx)

	r.InitClusters(ctx)

	go func(ctx context.Context) {
		defer log.RecordPanic()

		{
			type clusterSetupRes struct {
				cluster *cluster.Cluster
				err     error
				cert    *tls.Certificate
			}
			resCh := make(chan clusterSetupRes)
			for _, cr := range r.clusters {
				go func(cr *cluster.Cluster) {
					defer log.RecordPanic()
					if err := cr.Connect(ctx); err != nil {
						log.Errorf("Failed to connect cluster %s to server %q: %v", cr.ID(), cr.Options().Server, err)
						resCh <- clusterSetupRes{cluster: cr, err: err}
						return
					}
					cert, err := r.RequestClusterCert(ctx, cr)
					if err != nil {
						log.Errorf("Failed to request certificate for cluster %s: %v", cr.ID(), err)
						resCh <- clusterSetupRes{cluster: cr, err: err}
						return
					}
					resCh <- clusterSetupRes{cluster: cr, cert: cert}
				}(cr)
			}
			for range len(r.clusters) {
				select {
				case res := <-resCh:
					r.certificates[res.cluster.Name()] = res.cert
				case <-ctx.Done():
					return
				}
			}
		}

		r.listener.TLSConfig.Store(r.PatchTLSWithClusterCertificates(r.tlsConfig))

		if !r.Config.Tunneler.Enable {
			strPort := strconv.Itoa((int)(r.getPublicPort()))
			pubAddr := net.JoinHostPort(r.Config.PublicHost, strPort)
			localAddr := net.JoinHostPort(r.Config.Host, strconv.Itoa((int)(r.Config.Port)))
			log.TrInfof("info.server.public.at", pubAddr, localAddr, r.getCertCount())
		}

		log.TrInfof("info.wait.first.sync")
		r.InitSynchronizer(ctx)

		r.EnableClusterAll(ctx)
	}(ctx)

	code := r.ListenSignals(ctx, cancel)
	if code != 0 {
		log.TrErrorf("program.exited", code)
		log.TrErrorf("error.exit.please.read.faq")
		if runtime.GOOS == "windows" && !r.Config.Advanced.DoNotOpenFAQOnWindows {
			// log.TrWarnf("warn.exit.detected.windows.open.browser")
			// cmd := exec.Command("cmd", "/C", "start", "https://cdn.crashmc.com/https://github.com/LiterMC/go-openbmclapi?tab=readme-ov-file#faq")
			// cmd.Start()
			time.Sleep(time.Hour)
		}
	}
	os.Exit(code)
}
