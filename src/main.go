package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
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
	"syscall"
	"time"
)

var (
	SyncFileInterval  = time.Minute * 10
	KeepAliveInterval = time.Second * 60
)

type Config struct {
	Debug           bool   `json:"debug"`
	ShowServeInfo   bool   `json:"show_serve_info"`
	Nohttps         bool   `json:"nohttps"`
	PublicHost      string `json:"public_host"`
	PublicPort      uint16 `json:"public_port"`
	Port            uint16 `json:"port"`
	ClusterId       string `json:"cluster_id"`
	ClusterSecret   string `json:"cluster_secret"`
	UseOss          bool   `json:"use_oss"`
	OssRedirectBase string `json:"oss_redirect_base"`
	Hijack          bool   `json:"hijack"`
	HijackPort      uint16 `json:"hijack_port"`
	AntiHijackDNS   string `json:"anti_hijack_dns"`
}

var config Config

func readConfig() {
	const configPath = "config.json"

	config = Config{
		Debug:           false,
		ShowServeInfo:   false,
		Nohttps:         false,
		PublicHost:      "example.com",
		PublicPort:      8080,
		Port:            4000,
		ClusterId:       "${CLUSTER_ID}",
		ClusterSecret:   "${CLUSTER_SECRET}",
		UseOss:          false,
		OssRedirectBase: "https://oss.example.com/base/paths",
		Hijack:          false,
		HijackPort:      8090,
		AntiHijackDNS:   "8.8.8.8:53",
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logError("Cannot read config:", err)
			os.Exit(1)
		}
		logError("Config file not exists, create one")
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

const ossMirrorDir = "oss_mirror"

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
	redirectBase := ""
	if config.UseOss {
		redirectBase = config.OssRedirectBase
	}
	cluster := NewCluster(ctx, cacheDir,
		config.PublicHost, config.PublicPort,
		config.ClusterId, config.ClusterSecret,
		fmt.Sprintf("%s:%d", "0.0.0.0", config.Port),
		config.Nohttps, dialer,
		redirectBase,
	)

	logInfof("Starting Go-OpenBmclApi v%s (%s)", ClusterVersion, BuildVersion)

	{
		logInfof("Fetching file list")
		fl := cluster.GetFileList()
		if fl == nil {
			logError("Cluster filelist is nil, exit")
			os.Exit(1)
		}
		cluster.SyncFiles(fl, ctx)
	}

	if config.UseOss {
		createOssMirrorDir()
		assertOSS()
		go func() {
			ticker := time.NewTicker(time.Minute * 10)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					assertOSS()
				}
			}
		}()
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
		shutCtx, _ := context.WithTimeout(ctx, 16*time.Second)
		logWarn("Closing server ...")
		shutExit := make(chan struct{}, 0)
		go hjServer.Shutdown(shutCtx)
		go func() {
			defer close(shutExit)
			defer cancel()
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

func createOssMirrorDir() {
	logInfof("Creating %s", ossMirrorDir)
	if err := os.MkdirAll(ossMirrorDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", ossMirrorDir, err)
		os.Exit(2)
	}
	downloadDir := filepath.Join(ossMirrorDir, "download")
	os.RemoveAll(downloadDir)
	if err := os.Mkdir(downloadDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", downloadDir, err)
		os.Exit(2)
	}
	measureDir := filepath.Join(ossMirrorDir, "measure")
	if err := os.Mkdir(measureDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", measureDir, err)
		os.Exit(2)
	}
	for i := 0; i < 0x100; i++ {
		d := hex.EncodeToString([]byte{(byte)(i)})
		o := filepath.Join(cacheDir, d)
		t := filepath.Join(downloadDir, d)
		os.Mkdir(o, 0755)
		if err := os.Symlink(filepath.Join("..", "..", o), t); err != nil {
			logErrorf("Cannot create OSS mirror cache symlink %q: %v", t, err)
			os.Exit(2)
		}
	}
	logDebug("Creating measure files")
	buf := make([]byte, 200 * 1024 * 1024)
	for i := 1; i <= 200; i++ {
		size := i * 1024 * 1024
		t := filepath.Join(measureDir, strconv.Itoa(i))
		if stat, err := os.Stat(t); err == nil {
			x := stat.Size()
			if x == (int64)(size) {
				logDebug("Skipping", t)
				continue
			}
			logDebugf("File [%d] size %d does not match %d", i, x, size)
		}else{
			logDebugf("Cannot get stat of %s: %v", t, err)
		}
		logDebug("Writing", t)
		if err := os.WriteFile(t, buf[:size], 0644); err != nil {
			logErrorf("Cannot create OSS mirror measure file %q: %v", t, err)
			os.Exit(2)
		}
	}
	logDebug("Measure files created")
}

func assertOSS() {
	logInfo("Checking OSS ...")
	target, err := url.JoinPath(config.OssRedirectBase, "measure", "10")
	if err != nil {
		logError("Cannot check OSS server:", err)
		os.Exit(2)
	}
	res, err := http.Get(target)
	if err != nil {
		logErrorf("OSS check request failed %q: %v", target, err)
		os.Exit(2)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		logErrorf("OSS check request failed %q: %d %s", target, res.StatusCode, res.Status)
		os.Exit(2)
	}
	logDebug("reading OSS response")
	n, err := io.Copy(io.Discard, res.Body)
	if err != nil {
		logErrorf("OSS check request failed %q: %v", target, err)
		os.Exit(2)
	}
	if n != 10*1024*1024 {
		logErrorf("OSS check request failed %q: expected 10MB, but got %d bytes", target, n)
		os.Exit(2)
	}
}
