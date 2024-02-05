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
	"errors"
	"os"
	"strconv"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

type OSSItem struct {
	FolderPath        string `yaml:"folder_path"`
	RedirectBase      string `yaml:"redirect_base"`
	PreCreateMeasures bool   `yaml:"pre-create-measures"`
	Possibility       uint   `yaml:"possibility"`

	supportRange bool
	working      atomic.Bool
}

type ServeLimitConfig struct {
	Enable     bool `yaml:"enable"`
	MaxConn    int  `yaml:"max_conn"`
	UploadRate int  `yaml:"upload_rate"`
}

type DashboardConfig struct {
	Enable       bool   `yaml:"enable"`
	PwaName      string `yaml:"pwa-name"`
	PwaShortName string `yaml:"pwa-short_name"`
	PwaDesc      string `yaml:"pwa-description"`
}

type OSSConfig struct {
	Enable bool       `yaml:"enable"`
	List   []*OSSItem `yaml:"list"`
}

type HijackConfig struct {
	Enable        bool   `yaml:"enable"`
	ServerHost    string `yaml:"server_host"`
	ServerPort    uint16 `yaml:"server_port"`
	Path          string `yaml:"path"`
	AntiHijackDNS string `yaml:"anti_hijack_dns"`
}

type Config struct {
	Debug                bool             `yaml:"debug"`
	RecordServeInfo      bool             `yaml:"record_serve_info"`
	SkipFirstSync        bool             `yaml:"skip-first-sync"`
	ExitWhenDisconnected bool             `yaml:"exit-when-disconnected"`
	LogSlots             int              `yaml:"log-slots"`
	Byoc                 bool             `yaml:"byoc"`
	NoOpen               bool             `yaml:"noopen"`
	NoHeavyCheck         bool             `yaml:"no_heavy_check"`
	TrustedXForwardedFor bool             `yaml:"trusted-x-forwarded-for"`
	PublicHost           string           `yaml:"public_host"`
	PublicPort           uint16           `yaml:"public_port"`
	Port                 uint16           `yaml:"port"`
	ClusterId            string           `yaml:"cluster_id"`
	ClusterSecret        string           `yaml:"cluster_secret"`
	SyncInterval         int              `yaml:"sync_interval"`
	KeepaliveTimeout     int              `yaml:"keepalive_timeout"`
	DownloadMaxConn      int              `yaml:"download_max_conn"`
	UseGzip              bool             `yaml:"use_gzip"`
	ServeLimit           ServeLimitConfig `yaml:"serve_limit"`
	Dashboard            DashboardConfig  `yaml:"dashboard"`
	Oss                  OSSConfig        `yaml:"oss"`
	Hijack               HijackConfig     `yaml:"hijack"`
}

func (cfg *Config) applyWebManifest(manifest map[string]any) {
	if cfg.Dashboard.Enable {
		manifest["name"] = cfg.Dashboard.PwaName
		manifest["short_name"] = cfg.Dashboard.PwaShortName
		manifest["description"] = cfg.Dashboard.PwaDesc
	}
}

var defaultConfig = Config{
	Debug:                false,
	RecordServeInfo:      false,
	SkipFirstSync:        false,
	ExitWhenDisconnected: false,
	LogSlots:             7,
	Byoc:                 false,
	NoOpen:               false,
	NoHeavyCheck:         false,
	TrustedXForwardedFor: false,
	PublicHost:           "example.com",
	PublicPort:           8080,
	Port:                 4000,
	ClusterId:            "${CLUSTER_ID}",
	ClusterSecret:        "${CLUSTER_SECRET}",
	SyncInterval:         10,
	KeepaliveTimeout:     10,
	DownloadMaxConn:      64,
	UseGzip:              false,
	ServeLimit: ServeLimitConfig{
		Enable:     false,
		MaxConn:    16384,
		UploadRate: 1024 * 12, // 12MB
	},

	Dashboard: DashboardConfig{
		Enable:       true,
		PwaName:      "GoOpenBmclApi Dashboard",
		PwaShortName: "GOBA Dash",
		PwaDesc:      "Go-Openbmclapi Internal Dashboard",
	},

	Oss: OSSConfig{
		Enable: false,
		List: []*OSSItem{
			{
				FolderPath:        "oss_mirror",
				RedirectBase:      "https://oss.example.com/base/paths",
				PreCreateMeasures: false,
				Possibility:       0,
			},
		},
	},

	Hijack: HijackConfig{
		Enable:        false,
		ServerHost:    "",
		ServerPort:    8090,
		Path:          "__hijack",
		AntiHijackDNS: "8.8.8.8:53",
	},
}

func migrateConfig(data []byte, config *Config) {
	var oldConfig map[string]any
	if err := yaml.Unmarshal(data, &oldConfig); err != nil {
		return
	}
	if nohttps, ok := oldConfig["nohttps"].(bool); ok {
		config.Byoc = nohttps
	}
}

func readConfig() (config Config) {
	const configPath = "config.yaml"

	config = defaultConfig

	data, err := os.ReadFile(configPath)
	notexists := false
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logError("Cannot read config:", err)
			os.Exit(1)
		}
		logError("Config file not exists, create one")
		notexists = true
	} else {
		migrateConfig(data, &config)
		if err = yaml.Unmarshal(data, &config); err != nil {
			logError("Cannot parse config:", err)
			os.Exit(1)
		}
	}

	if data, err = yaml.Marshal(config); err != nil {
		logError("Cannot encode config:", err)
		os.Exit(1)
	}
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		logError("Cannot write config:", err)
		os.Exit(1)
	}
	if notexists {
		logError("Config file created, please edit it and start the program again")
		os.Exit(0xff)
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
		config.Byoc = byoc == "true"
	}
	switch noopen := os.Getenv("FORCE_NOOPEN"); noopen {
	case "true":
		config.NoOpen = true
	case "false":
		config.NoOpen = false
	}
	return
}
