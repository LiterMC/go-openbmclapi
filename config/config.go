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
	"encoding/json"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/LiterMC/go-openbmclapi/cache"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type Config struct {
	PublicHost           string `json:"public_host" yaml:"public-host"`
	PublicPort           uint16 `json:"public_port" yaml:"public-port"`
	Host                 string `json:"host" yaml:"host"`
	Port                 uint16 `json:"port" yaml:"port"`
	UseCert              bool   `json:"use_cert" yaml:"use-cert"`
	TrustedXForwardedFor bool   `json:"trusted_x_forwarded_for" yaml:"trusted-x-forwarded-for"`

	OnlyGcWhenStart   bool `json:"only_gc_when_start" yaml:"only-gc-when-start"`
	SyncInterval      int  `json:"sync_interval" yaml:"sync-interval"`
	DownloadMaxConn   int  `json:"download_max_conn" yaml:"download-max-conn"`
	MaxReconnectCount int  `json:"max_reconnect_count" yaml:"max-reconnect-count"`

	LogSlots       int  `json:"log_slots" yaml:"log-slots"`
	NoAccessLog    bool `json:"no_access_log" yaml:"no-access-log"`
	AccessLogSlots int  `json:"access_log_slots" yaml:"access-log-slots"`

	Clusters     map[string]ClusterOptions      `json:"clusters" yaml:"clusters"`
	Storages     []storage.StorageOption        `json:"storages" yaml:"storages"`
	Certificates []CertificateConfig            `json:"certificates" yaml:"certificates"`
	Tunneler     TunnelConfig                   `json:"tunneler" yaml:"tunneler"`
	Cache        CacheConfig                    `json:"cache" yaml:"cache"`
	ServeLimit   ServeLimitConfig               `json:"serve_limit" yaml:"serve-limit"`
	RateLimit    APIRateLimitConfig             `json:"api_rate_limit" yaml:"api-rate-limit"`
	Notification NotificationConfig             `json:"notification" yaml:"notification"`
	Dashboard    DashboardConfig                `json:"dashboard" yaml:"dashboard"`
	GithubAPI    GithubAPIConfig                `json:"github_api" yaml:"github-api"`
	Database     DatabaseConfig                 `json:"database" yaml:"database"`
	Hijack       HijackConfig                   `json:"hijack" yaml:"hijack"`
	WebdavUsers  map[string]*storage.WebDavUser `json:"webdav_users" yaml:"webdav-users"`
	Advanced     AdvancedConfig                 `json:"advanced" yaml:"advanced"`
}

func (cfg *Config) ApplyWebManifest(manifest map[string]any) {
	if cfg.Dashboard.Enable {
		manifest["name"] = cfg.Dashboard.PwaName
		manifest["short_name"] = cfg.Dashboard.PwaShortName
		manifest["description"] = cfg.Dashboard.PwaDesc
	}
}

func NewDefaultConfig() *Config {
	return &Config{
		PublicHost:           "",
		PublicPort:           443,
		Host:                 "0.0.0.0",
		Port:                 4000,
		UseCert:              false,
		TrustedXForwardedFor: false,

		OnlyGcWhenStart:   false,
		SyncInterval:      10,
		DownloadMaxConn:   16,
		MaxReconnectCount: 10,

		LogSlots:       7,
		NoAccessLog:    false,
		AccessLogSlots: 16,

		Clusters: map[string]ClusterOptions{},

		Storages: nil,

		Certificates: []CertificateConfig{},

		Tunneler: TunnelConfig{
			Enable:      false,
			TunnelProg:  "./path/to/tunnel/program",
			OutputRegex: `\bNATedAddr\s+(?P<host>[0-9.]+|\[[0-9a-f:]+\]):(?P<port>\d+)$`,
		},

		Cache: CacheConfig{
			Type:     "memory",
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
			Username:      "",
			Password:      "",
			PwaName:       "GoOpenBmclApi Dashboard",
			PwaShortName:  "GOBA Dash",
			PwaDesc:       "Go-Openbmclapi Internal Dashboard",
			NotifySubject: "mailto:user@example.com",
		},

		GithubAPI: GithubAPIConfig{
			UpdateCheckInterval: (utils.YAMLDuration)(time.Hour),
			Authorization:       "",
		},

		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    filepath.Join("data", "files.db"),
		},

		Hijack: HijackConfig{
			Enable:           false,
			EnableLocalCache: false,
			LocalCachePath:   "hijack_cache",
			RequireAuth:      false,
			AuthUsers: []UserItem{
				{
					Username: "example-username",
					Password: "example-password",
				},
			},
		},

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

func (config *Config) MarshalJSON() ([]byte, error) {
	type T Config
	return json.Marshal((*T)(config))
}

func (config *Config) UnmarshalJSON(data []byte) error {
	type T Config
	return json.Unmarshal(data, (*T)(config))
}

func (config *Config) UnmarshalText(data []byte) error {
	return yaml.Unmarshal(data, config)
}

func (config *Config) Clone() *Config {
	data, err := config.MarshalJSON()
	if err != nil {
		panic(err)
	}
	cloned := new(Config)
	if err := cloned.UnmarshalJSON(data); err != nil {
		panic(err)
	}
	return cloned
}
