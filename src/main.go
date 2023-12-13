package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

var (
	SyncFileInterval  = time.Minute * 10
	KeepAliveInterval = time.Second * 60
)

type Config struct {
	Debug            bool   `json:"debug"`
	ShowServeInfo    bool   `json:"show_serve_info"`
	IgnoreServeError bool   `json:"ignore_serve_error"`
	Nohttps          bool   `json:"nohttps"`
	PublicHost       string `json:"public_host"`
	PublicPort       uint16 `json:"public_port"`
	Port             uint16 `json:"port"`
	ClusterId        string `json:"cluster_id"`
	ClusterSecret    string `json:"cluster_secret"`
	Hijack           bool   `json:"hijack"`
	HijackPort       uint16 `json:"hijack_port"`
	AntiHijackDNS    string `json:"anti_hijack_dns"`
}

var config Config

func readConfig() {
	const configPath = "config.json"
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logError("Cannot read config:", err)
			os.Exit(1)
		}
		logError("Config file not exists, create one")
		config = Config{
			Debug:            false,
			ShowServeInfo:    false,
			IgnoreServeError: true,
			Nohttps:          false,
			PublicHost:       "example.com",
			PublicPort:       8080,
			Port:             4000,
			ClusterId:        "${CLUSTER_ID}",
			ClusterSecret:    "${CLUSTER_SECRET}",
			Hijack:           false,
			HijackPort:       8090,
			AntiHijackDNS:    "8.8.8.8:53",
		}
	} else if err = json.Unmarshal(data, &config); err != nil {
		logError("Cannot parse config:", err)
		os.Exit(1)
	}

	if data, err = json.MarshalIndent(config, "", "  "); err != nil {
		logError("Cannot encode config:", err)
		os.Exit(1)
	}
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		logError("Cannot write config:", err)
		os.Exit(1)
	}

	if os.Getenv("DEBUG") == "true" {
		config.Debug = true
	}
	if v := os.Getenv("CLUSTER_IP"); v != "" {
		config.PublicHost = v
	}
	if v := os.Getenv("CLUSTER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			logErrorf("Cannot parse CLUSTER_PORT %q: %v", v, err)
		} else {
			config.Port = (uint16)(n)
		}
	}
	if v := os.Getenv("CLUSTER_PUBLIC_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			logErrorf("Cannot parse CLUSTER_PUBLIC_PORT %q: %v", v, err)
		} else {
			config.PublicPort = (uint16)(n)
		}
	}
	if v := os.Getenv("CLUSTER_ID"); v != "" {
		config.ClusterId = v
	}
	if v := os.Getenv("CLUSTER_SECRET"); v != "" {
		config.ClusterSecret = v
	}
	if byoc := os.Getenv("CLUSTER_BYOC"); byoc != "" {
		config.Nohttps = byoc == "true"
	}
}

const cacheDir = "cache"

var hijackPath = filepath.Join(cacheDir, "__hijack")

func main() {
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

	exitCh := make(chan struct{}, 0)
	ctx, cancel := context.WithCancel(bgctx)

	readConfig()

	var (
		dialer   *net.Dialer
		hjproxy  *HjProxy
		hjServer *http.Server
	)
	if config.Hijack {
		dialer = getDialerWithDNS(config.AntiHijackDNS)
		hjproxy = NewHjProxy(dialer, hijackPath)
		hjServer = &http.Server{
			Addr:    fmt.Sprintf("0.0.0.0:%d", config.HijackPort),
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
	cluster := NewCluster(ctx, cacheDir,
		config.PublicHost, config.PublicPort,
		config.ClusterId, config.ClusterSecret, VERSION,
		fmt.Sprintf("%s:%d", "0.0.0.0", config.Port),
		config.Nohttps, dialer)

	logInfof("Starting Go-OpenBmclApi v%s", VERSION)

	{
		logInfof("Fetching file list")
		fl := cluster.GetFileList()
		if fl == nil {
			logError("Cluster filelist is nil, exit")
			os.Exit(1)
		}
		cluster.SyncFiles(fl, ctx)
	}

	if !cluster.Connect() {
		os.Exit(1)
	}

	var certFile, keyFile string
	if !config.Nohttps {
		pair, err := cluster.RequestCert()
		if err != nil {
			logError("Error when requesting cert key pair:", err)
			os.Exit(1)
		}
		certFile, keyFile, err = pair.SaveAsFile()
		if err != nil {
			logError("Error when saving cert key pair:", err)
			os.Exit(1)
		}
	}

	go func() {
		defer close(exitCh)
		logInfof("Server start at %q", cluster.Server.Addr)
		var err error
		if !config.Nohttps {
			err = cluster.Server.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = cluster.Server.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logError("Error on server:", err)
			os.Exit(1)
		}
	}()

	if err := cluster.Enable(); err != nil {
		logError("Cannot enable cluster:", err)
		os.Exit(1)
	}

	createInterval(ctx, func() {
		fl := cluster.GetFileList()
		if fl == nil {
			logError("Cannot get cluster file list, exit")
			os.Exit(1)
		}
		cluster.SyncFiles(fl, ctx)
	}, SyncFileInterval)

	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	select {
	case <-exitCh:
		return
	case s := <-signalCh:
		shutCtx, cancelShut := context.WithTimeout(bgctx, 16*time.Second)
		logWarn("Closing server ...")
		cancel()
		shutExit := make(chan struct{}, 0)
		go hjServer.Shutdown(shutCtx)
		go func() {
			defer close(shutExit)
			defer cancelShut()
			cluster.Disable()
			cluster.Server.Shutdown(shutCtx)
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
