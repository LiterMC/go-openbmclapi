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

	"gopkg.in/yaml.v3"
)

type ServeLimitConfig struct {
	Enable     bool `yaml:"enable"`
	MaxConn    int  `yaml:"max-conn"`
	UploadRate int  `yaml:"upload-rate"`
}

type DashboardConfig struct {
	Enable       bool   `yaml:"enable"`
	PwaName      string `yaml:"pwa-name"`
	PwaShortName string `yaml:"pwa-short_name"`
	PwaDesc      string `yaml:"pwa-description"`
}

type WebDavUser struct {
	EndPoint string `yaml:"endpoint,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type Config struct {
	Debug                bool   `yaml:"debug"`
	RecordServeInfo      bool   `yaml:"record-serve-info"`
	SkipFirstSync        bool   `yaml:"skip-first-sync"`
	ExitWhenDisconnected bool   `yaml:"exit-when-disconnected"`
	LogSlots             int    `yaml:"log-slots"`
	Byoc                 bool   `yaml:"byoc"`
	NoOpen               bool   `yaml:"noopen"`
	NoHeavyCheck         bool   `yaml:"no-heavy-check"`
	TrustedXForwardedFor bool   `yaml:"trusted-x-forwarded-for"`
	PublicHost           string `yaml:"public-host"`
	PublicPort           uint16 `yaml:"public-port"`
	Port                 uint16 `yaml:"port"`
	ClusterId            string `yaml:"cluster-id"`
	ClusterSecret        string `yaml:"cluster-secret"`
	SyncInterval         int    `yaml:"sync-interval"`
	KeepaliveTimeout     int    `yaml:"keepalive-timeout"`
	DownloadMaxConn      int    `yaml:"download-max-conn"`

	ServeLimit  ServeLimitConfig       `yaml:"serve-limit"`
	Dashboard   DashboardConfig        `yaml:"dashboard"`
	Storages    []StorageOption        `yaml:"storages"`
	WebdavUsers map[string]*WebDavUser `yaml:"webdav-users"`
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

	Storages: nil,

	WebdavUsers: map[string]*WebDavUser{
		"example-user": &WebDavUser{
			EndPoint: "https://webdav.example.com/path/to/endpoint/",
			Username: "example-username",
			Password: "example-password",
		},
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
	if v, ok := oldConfig["record_serve_info"].(bool); ok {
		config.RecordServeInfo = v
	}
	if v, ok := oldConfig["no_heavy_check"].(bool); ok {
		config.NoHeavyCheck = v
	}
	if v, ok := oldConfig["public_host"].(string); ok {
		config.PublicHost = v
	}
	if v, ok := oldConfig["public_port"].(int); ok {
		config.PublicPort = (uint16)(v)
	}
	if v, ok := oldConfig["cluster_id"].(string); ok {
		config.ClusterId = v
	}
	if v, ok := oldConfig["cluster_secret"].(string); ok {
		config.ClusterSecret = v
	}
	if v, ok := oldConfig["sync_interval"].(int); ok {
		config.SyncInterval = v
	}
	if v, ok := oldConfig["keepalive_timeout"].(int); ok {
		config.KeepaliveTimeout = v
	}
	if v, ok := oldConfig["download_max_conn"].(int); ok {
		config.DownloadMaxConn = v
	}
	if limit, ok := oldConfig["serve_limit"].(map[string]any); ok {
		var sl ServeLimitConfig
		if sl.Enable, ok = limit["enable"].(bool); !ok {
			goto SKIP_SERVE_LIMIT
		}
		if sl.MaxConn, ok = limit["max_conn"].(int); !ok {
			goto SKIP_SERVE_LIMIT
		}
		if sl.UploadRate, ok = limit["upload_rate"].(int); !ok {
			goto SKIP_SERVE_LIMIT
		}
		config.ServeLimit = sl
	SKIP_SERVE_LIMIT:
	}

	if oss, ok := oldConfig["oss"].(map[string]any); ok && oss["enable"] == true {
		var storages []StorageOption
		logInfo("Migrate old oss config to latest format")
		if list, ok := oss["list"].([]any); ok {
			for _, v := range list {
				if item, ok := v.(map[string]any); ok {
					var (
						stItem   StorageOption
						mountOpt = new(MountStorageOption)
					)
					stItem.Type = StorageMount
					folderPath, ok := item["folder_path"].(string)
					if !ok {
						continue
					}
					mountOpt.Path = folderPath
					redirectBase, ok := item["redirect_base"].(string)
					if !ok {
						continue
					}
					mountOpt.RedirectBase = redirectBase
					preGenMeasures, ok := item["pre-create-measures"].(bool)
					if ok {
						mountOpt.PreGenMeasures = preGenMeasures
					}
					weight, ok := item["possibility"].(int)
					if !ok {
						weight = 100
					}
					stItem.Weight = (uint)(weight)
					stItem.Data = mountOpt
					storages = append(storages, stItem)
				}
			}
		}
		config.Storages = storages
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
		if len(config.Storages) == 0 {
			config.Storages = []StorageOption{
				{
					Type:   StorageLocal,
					Weight: 100,
					Data: &LocalStorageOption{
						CachePath: "cache",
					},
				},
			}
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
