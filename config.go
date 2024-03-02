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
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/LiterMC/go-openbmclapi/cache"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type UserItem struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type AdvancedConfig struct {
	DebugLog             bool `yaml:"debug-log"`
	SocketIOLog          bool `yaml:"socket-io-log"`
	NoHeavyCheck         bool `yaml:"no-heavy-check"`
	NoGC                 bool `yaml:"no-gc"`
	HeavyCheckInterval   int  `yaml:"heavy-check-interval"`
	KeepaliveTimeout     int  `yaml:"keepalive-timeout"`
	SkipFirstSync        bool `yaml:"skip-first-sync"`
	SkipSignatureCheck   bool `yaml:"skip-signature-check"`
	ExitWhenDisconnected bool `yaml:"exit-when-disconnected"`
	NoFastEnable         bool `yaml:"no-fast-enable"`
	WaitBeforeEnable     int  `yaml:"wait-before-enable"`

	DoNotRedirectHTTPSToSecureHostname bool `yaml:"do-NOT-redirect-https-to-SECURE-hostname"`
}

type CertificateConfig struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type ServeLimitConfig struct {
	Enable     bool `yaml:"enable"`
	MaxConn    int  `yaml:"max-conn"`
	UploadRate int  `yaml:"upload-rate"`
}

type HijackConfig struct {
	Enable      bool       `yaml:"enable"`
	RequireAuth bool       `yaml:"require-auth"`
	AuthUsers   []UserItem `yaml:"auth-users"`
}

type CacheConfig struct {
	Type string `yaml:"type"`
	Data any    `yaml:"data,omitempty"`

	newCache func() cache.Cache `yaml:"-"`
}

func (c *CacheConfig) UnmarshalYAML(n *yaml.Node) (err error) {
	var cfg struct {
		Type string        `yaml:"type"`
		Data utils.RawYAML `yaml:"data,omitempty"`
	}
	if err = n.Decode(&cfg); err != nil {
		return
	}
	c.Type = cfg.Type
	c.Data = nil
	switch strings.ToLower(c.Type) {
	case "no", "off", "disabled", "nocache", "no-cache":
		c.newCache = func() cache.Cache { return cache.NoCache }
	case "mem", "memory", "inmem":
		c.newCache = func() cache.Cache { return cache.NewInMemCache() }
	case "redis":
		opt := new(cache.RedisOptions)
		if err = cfg.Data.Decode(opt); err != nil {
			return
		}
		c.Data = opt
		c.newCache = func() cache.Cache { return cache.NewRedisCache(opt.ToRedis()) }
	default:
		return fmt.Errorf("Unexpected cache type %q", c.Type)
	}
	return nil
}

type DashboardConfig struct {
	Enable       bool   `yaml:"enable"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	PwaName      string `yaml:"pwa-name"`
	PwaShortName string `yaml:"pwa-short_name"`
	PwaDesc      string `yaml:"pwa-description"`
}

type Config struct {
	LogSlots             int    `yaml:"log-slots"`
	NoAccessLog          bool   `yaml:"no-access-log"`
	AccessLogSlots       int    `yaml:"access-log-slots"`
	Byoc                 bool   `yaml:"byoc"`
	UseCert              bool   `yaml:"use-cert"`
	TrustedXForwardedFor bool   `yaml:"trusted-x-forwarded-for"`
	PublicHost           string `yaml:"public-host"`
	PublicPort           uint16 `yaml:"public-port"`
	Port                 uint16 `yaml:"port"`
	ClusterId            string `yaml:"cluster-id"`
	ClusterSecret        string `yaml:"cluster-secret"`
	SyncInterval         int    `yaml:"sync-interval"`
	OnlyGcWhenStart      bool   `yaml:"only-gc-when-start"`
	DownloadMaxConn      int    `yaml:"download-max-conn"`

	Certificates []CertificateConfig            `yaml:"certificates"`
	Cache        CacheConfig                    `yaml:"cache"`
	ServeLimit   ServeLimitConfig               `yaml:"serve-limit"`
	Dashboard    DashboardConfig                `yaml:"dashboard"`
	Hijack       HijackConfig                   `yaml:"hijack"`
	Storages     []storage.StorageOption        `yaml:"storages"`
	WebdavUsers  map[string]*storage.WebDavUser `yaml:"webdav-users"`
	Advanced     AdvancedConfig                 `yaml:"advanced"`
}

func (cfg *Config) applyWebManifest(manifest map[string]any) {
	if cfg.Dashboard.Enable {
		manifest["name"] = cfg.Dashboard.PwaName
		manifest["short_name"] = cfg.Dashboard.PwaShortName
		manifest["description"] = cfg.Dashboard.PwaDesc
	}
}

var defaultConfig = Config{
	LogSlots:             7,
	NoAccessLog:          false,
	AccessLogSlots:       16,
	Byoc:                 false,
	TrustedXForwardedFor: false,
	PublicHost:           "",
	PublicPort:           0,
	Port:                 4000,
	ClusterId:            "${CLUSTER_ID}",
	ClusterSecret:        "${CLUSTER_SECRET}",
	SyncInterval:         10,
	OnlyGcWhenStart:      false,
	DownloadMaxConn:      16,

	Certificates: []CertificateConfig{
		{
			Cert: "/path/to/cert.pem",
			Key:  "/path/to/key.pem",
		},
	},

	Cache: CacheConfig{
		Type:     "inmem",
		newCache: func() cache.Cache { return cache.NewInMemCache() },
	},

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

	Hijack: HijackConfig{
		Enable:      false,
		RequireAuth: false,
		AuthUsers: []UserItem{
			{
				Username: "example-username",
				Password: "example-password",
			},
		},
	},

	Storages: nil,

	WebdavUsers: map[string]*storage.WebDavUser{},

	Advanced: AdvancedConfig{
		DebugLog:             false,
		NoHeavyCheck:         false,
		NoGC:                 false,
		HeavyCheckInterval:   120,
		KeepaliveTimeout:     10,
		SkipFirstSync:        false,
		ExitWhenDisconnected: false,
		NoFastEnable:         false,
		WaitBeforeEnable:     0,
	},
}

func migrateConfig(data []byte, config *Config) {
	var oldConfig map[string]any
	if err := yaml.Unmarshal(data, &oldConfig); err != nil {
		return
	}

	// if nohttps, ok := oldConfig["nohttps"].(bool); ok {
	// 	config.Byoc = nohttps
	// }
	// if v, ok := oldConfig["record_serve_info"].(bool); ok {
	// 	config.RecordServeInfo = v
	// }
	// if v, ok := oldConfig["no_heavy_check"].(bool); ok {
	// 	config.NoHeavyCheck = v
	// }
	// if v, ok := oldConfig["public_host"].(string); ok {
	// 	config.PublicHost = v
	// }
	// if v, ok := oldConfig["public_port"].(int); ok {
	// 	config.PublicPort = (uint16)(v)
	// }
	// if v, ok := oldConfig["cluster_id"].(string); ok {
	// 	config.ClusterId = v
	// }
	// if v, ok := oldConfig["cluster_secret"].(string); ok {
	// 	config.ClusterSecret = v
	// }
	// if v, ok := oldConfig["sync_interval"].(int); ok {
	// 	config.SyncInterval = v
	// }
	// if v, ok := oldConfig["keepalive_timeout"].(int); ok {
	// 	config.KeepaliveTimeout = v
	// }
	// if v, ok := oldConfig["download_max_conn"].(int); ok {
	// 	config.DownloadMaxConn = v
	// }
	// if limit, ok := oldConfig["serve_limit"].(map[string]any); ok {
	// 	var sl ServeLimitConfig
	// 	if sl.Enable, ok = limit["enable"].(bool); !ok {
	// 		goto SKIP_SERVE_LIMIT
	// 	}
	// 	if sl.MaxConn, ok = limit["max_conn"].(int); !ok {
	// 		goto SKIP_SERVE_LIMIT
	// 	}
	// 	if sl.UploadRate, ok = limit["upload_rate"].(int); !ok {
	// 		goto SKIP_SERVE_LIMIT
	// 	}
	// 	config.ServeLimit = sl
	// SKIP_SERVE_LIMIT:
	// }

	// if oss, ok := oldConfig["oss"].(map[string]any); ok && oss["enable"] == true {
	// 	var storages []StorageOption
	// 	logInfo("Migrate old oss config to latest format")
	// 	if list, ok := oss["list"].([]any); ok {
	// 		for _, v := range list {
	// 			if item, ok := v.(map[string]any); ok {
	// 				var (
	// 					stItem   StorageOption
	// 					mountOpt = new(MountStorageOption)
	// 				)
	// 				stItem.Type = StorageMount
	// 				folderPath, ok := item["folder_path"].(string)
	// 				if !ok {
	// 					continue
	// 				}
	// 				mountOpt.Path = folderPath
	// 				redirectBase, ok := item["redirect_base"].(string)
	// 				if !ok {
	// 					continue
	// 				}
	// 				mountOpt.RedirectBase = redirectBase
	// 				preGenMeasures, ok := item["pre-create-measures"].(bool)
	// 				if ok {
	// 					mountOpt.PreGenMeasures = preGenMeasures
	// 				}
	// 				weight, ok := item["possibility"].(int)
	// 				if !ok {
	// 					weight = 100
	// 				}
	// 				stItem.Weight = (uint)(weight)
	// 				stItem.Data = mountOpt
	// 				storages = append(storages, stItem)
	// 			}
	// 		}
	// 	}
	// 	config.Storages = storages
	// }

	if v, ok := oldConfig["debug"].(bool); ok {
		config.Advanced.DebugLog = v
	}
	if v, ok := oldConfig["skip-first-sync"].(bool); ok {
		config.Advanced.SkipFirstSync = v
	}
	if v, ok := oldConfig["exit-when-disconnected"].(bool); ok {
		config.Advanced.ExitWhenDisconnected = v
	}
	if v, ok := oldConfig["no-heavy-check"].(bool); ok {
		config.Advanced.NoHeavyCheck = v
	}
	if v, ok := oldConfig["keepalive-timeout"].(int); ok {
		config.Advanced.KeepaliveTimeout = v
	}
}

func readConfig() (config Config) {
	const configPath = "config.yaml"

	config = defaultConfig

	data, err := os.ReadFile(configPath)
	notexists := false
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Error("Cannot read config:", err)
			osExit(1)
		}
		log.Error("Config file not exists, create one")
		notexists = true
	} else {
		migrateConfig(data, &config)
		if err = yaml.Unmarshal(data, &config); err != nil {
			log.Error("Cannot parse config:", err)
			osExit(1)
		}
		if len(config.Storages) == 0 {
			config.Storages = []storage.StorageOption{
				{
					BasicStorageOption: storage.BasicStorageOption{
						Id:     "local",
						Type:   storage.StorageLocal,
						Weight: 100,
					},
					Data: &storage.LocalStorageOption{
						CachePath: "cache",
					},
				},
			}
		}
		if len(config.WebdavUsers) == 0 {
			config.WebdavUsers["example-user"] = &storage.WebDavUser{
				EndPoint: "https://webdav.example.com/path/to/endpoint/",
				Username: "example-username",
				Password: "example-password",
			}
		}
		ids := make(map[string]int, len(config.Storages))
		for i, s := range config.Storages {
			if s.Id == "" {
				s.Id = fmt.Sprintf("storage-%d", i)
				config.Storages[i].Id = s.Id
			}
			if j, ok := ids[s.Id]; ok {
				log.Errorf("Duplicated storage id %q at [%d] and [%d], please edit the config.", s.Id, i, j)
				osExit(1)
			}
			ids[s.Id] = i
		}
	}

	for _, so := range config.Storages {
		switch opt := so.Data.(type) {
		case *storage.WebDavStorageOption:
			if alias := opt.Alias; alias != "" {
				user, ok := config.WebdavUsers[alias]
				if !ok {
					log.Errorf("Web dav user %q does not exists", alias)
					osExit(1)
				}
				opt.AliasUser = user
				var end *url.URL
				if end, err = url.Parse(opt.AliasUser.EndPoint); err != nil {
					return
				}
				if opt.EndPoint != "" {
					var full *url.URL
					if full, err = end.Parse(opt.EndPoint); err != nil {
						return
					}
					opt.FullEndPoint = full.String()
				} else {
					opt.FullEndPoint = opt.AliasUser.EndPoint
				}
			} else {
				opt.FullEndPoint = opt.EndPoint
			}
		}
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err = encoder.Encode(config); err != nil {
		log.Error("Cannot encode config:", err)
		osExit(1)
	}
	if err = os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		log.Error("Cannot write config:", err)
		osExit(1)
	}
	if notexists {
		log.Error("Config file created, please edit it and start the program again")
		osExit(0xff)
	}

	if os.Getenv("DEBUG") == "true" {
		config.Advanced.DebugLog = true
	}
	if v := os.Getenv("CLUSTER_IP"); v != "" {
		config.PublicHost = v
	}
	if v := os.Getenv("CLUSTER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			log.Errorf("Cannot parse CLUSTER_PORT %q: %v", v, err)
		} else {
			config.Port = (uint16)(n)
		}
	}
	if v := os.Getenv("CLUSTER_PUBLIC_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			log.Errorf("Cannot parse CLUSTER_PUBLIC_PORT %q: %v", v, err)
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
	return
}

type OpenbmclapiAgentSyncConfig struct {
	Source      string `json:"source"`
	Concurrency int    `json:"concurrency"`
}

type OpenbmclapiAgentConfig struct {
	Sync OpenbmclapiAgentSyncConfig `json:"sync"`
}
