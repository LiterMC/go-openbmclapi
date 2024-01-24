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
	FolderPath     string `yaml:"folder_path"`
	RedirectBase   string `yaml:"redirect_base"`
	SkipMeasureGen bool   `yaml:"skip_measure_gen"`

	supportRange bool
	working      atomic.Bool
}

type ServeLimitConfig struct {
	Enable     bool `yaml:"enable"`
	MaxConn    int  `yaml:"max_conn"`
	UploadRate int  `yaml:"upload_rate"`
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
	Debug           bool             `yaml:"debug"`
	RecordServeInfo bool             `yaml:"record_serve_info"`
	Nohttps         bool             `yaml:"nohttps"`
	NoOpen          bool             `yaml:"noopen"`
	PublicHost      string           `yaml:"public_host"`
	PublicPort      uint16           `yaml:"public_port"`
	Port            uint16           `yaml:"port"`
	ClusterId       string           `yaml:"cluster_id"`
	ClusterSecret   string           `yaml:"cluster_secret"`
	SyncInterval    int              `yaml:"sync_interval"`
	DownloadMaxConn int              `yaml:"download_max_conn"`
	ServeLimit      ServeLimitConfig `yaml:"serve_limit"`
	Oss             OSSConfig        `yaml:"oss"`
	Hijack          HijackConfig     `yaml:"hijack_port"`
}

func readConfig() (config Config) {
	const configPath = "config.yaml"

	config = Config{
		Debug:           false,
		RecordServeInfo: false,
		Nohttps:         false,
		PublicHost:      "example.com",
		PublicPort:      8080,
		Port:            4000,
		ClusterId:       "${CLUSTER_ID}",
		ClusterSecret:   "${CLUSTER_SECRET}",
		SyncInterval:    10,
		DownloadMaxConn: 64,
		ServeLimit: ServeLimitConfig{
			Enable:     false,
			MaxConn:    16384,
			UploadRate: 1024 * 12, // 12MB
		},

		Oss: OSSConfig{
			Enable: false,
			List: []*OSSItem{
				{
					FolderPath:     "oss_mirror",
					RedirectBase:   "https://oss.example.com/base/paths",
					SkipMeasureGen: false,
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

	data, err := os.ReadFile(configPath)
	notexists := false
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logError("Cannot read config:", err)
			os.Exit(1)
		}
		logError("Config file not exists, create one")
		notexists = true
	} else if err = yaml.Unmarshal(data, &config); err != nil {
		logError("Cannot parse config:", err)
		os.Exit(1)
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
		config.Nohttps = byoc == "true"
	}
	switch noopen := os.Getenv("FORCE_NOOPEN"); noopen {
	case "true":
		config.NoOpen = true
	case "false":
		config.NoOpen = false
	}
	return
}
