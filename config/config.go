/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2024 Kevin Z <zyxkad@gmail.com>
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

package config

import (
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/LiterMC/go-openbmclapi/cache"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type Config struct {
	PublicHost           string `yaml:"public-host"`
	PublicPort           uint16 `yaml:"public-port"`
	Host                 string `yaml:"host"`
	Port                 uint16 `yaml:"port"`
	Byoc                 bool   `yaml:"byoc"`
	UseCert              bool   `yaml:"use-cert"`
	TrustedXForwardedFor bool   `yaml:"trusted-x-forwarded-for"`

	OnlyGcWhenStart   bool `yaml:"only-gc-when-start"`
	SyncInterval      int  `yaml:"sync-interval"`
	DownloadMaxConn   int  `yaml:"download-max-conn"`
	MaxReconnectCount int  `yaml:"max-reconnect-count"`

	LogSlots       int  `yaml:"log-slots"`
	NoAccessLog    bool `yaml:"no-access-log"`
	AccessLogSlots int  `yaml:"access-log-slots"`

	Clusters     map[string]ClusterOptions      `yaml:"clusters"`
	Certificates []CertificateConfig            `yaml:"certificates"`
	Tunneler     TunnelConfig                   `yaml:"tunneler"`
	Cache        CacheConfig                    `yaml:"cache"`
	ServeLimit   ServeLimitConfig               `yaml:"serve-limit"`
	RateLimit    APIRateLimitConfig             `yaml:"api-rate-limit"`
	Notification NotificationConfig             `yaml:"notification"`
	Dashboard    DashboardConfig                `yaml:"dashboard"`
	GithubAPI    GithubAPIConfig                `yaml:"github-api"`
	Database     DatabaseConfig                 `yaml:"database"`
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

func NewDefaultConfig() *Config {
	return &Config{
		PublicHost:           "",
		PublicPort:           0,
		Host:                 "0.0.0.0",
		Port:                 4000,
		Byoc:                 false,
		TrustedXForwardedFor: false,

		OnlyGcWhenStart:   false,
		SyncInterval:      10,
		DownloadMaxConn:   16,
		MaxReconnectCount: 10,

		LogSlots:       7,
		NoAccessLog:    false,
		AccessLogSlots: 16,

		Clusters: map[string]ClusterOptions{},

		Certificates: []CertificateConfig{},

		Tunneler: TunnelConfig{
			Enable:        false,
			TunnelProg:    "./path/to/tunnel/program",
			OutputRegex:   `\bNATedAddr\s+(?P<host>[0-9.]+|\[[0-9a-f:]+\]):(?P<port>\d+)$`,
			TunnelTimeout: 0,
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

		RateLimit: APIRateLimitConfig{
			Anonymous: limited.RateLimit{
				PerMin:  10,
				PerHour: 120,
			},
			Logged: limited.RateLimit{
				PerMin:  120,
				PerHour: 6000,
			},
		},

		Notification: NotificationConfig{
			EnableEmail:         false,
			EmailSMTP:           "smtp.example.com:25",
			EmailSMTPEncryption: "tls",
			EmailSender:         "noreply@example.com",
			EmailSenderPassword: "example-password",
			EnableWebhook:       true,
		},

		Dashboard: DashboardConfig{
			Enable:        true,
			PwaName:       "GoOpenBmclApi Dashboard",
			PwaShortName:  "GOBA Dash",
			PwaDesc:       "Go-Openbmclapi Internal Dashboard",
			NotifySubject: "mailto:user@example.com",
		},

		GithubAPI: GithubAPIConfig{
			UpdateCheckInterval: (utils.YAMLDuration)(time.Hour),
		},

		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    filepath.Join("data", "files.db"),
		},

		Hijack: HijackConfig{
			Enable:           false,
			RequireAuth:      false,
			EnableLocalCache: false,
			LocalCachePath:   "hijack_cache",
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
			DebugLog:           false,
			NoHeavyCheck:       false,
			NoGC:               false,
			HeavyCheckInterval: 120,
			KeepaliveTimeout:   10,
			NoFastEnable:       false,
			WaitBeforeEnable:   0,
		},
	}
}

func (config *Config) UnmarshalText(data []byte) error {
	return yaml.Unmarshal(data, config)
}
