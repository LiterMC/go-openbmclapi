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
	"errors"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/LiterMC/go-openbmclapi/cache"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type ClusterOptions struct {
	Id                 string   `json:"id" yaml:"id"`
	Secret             string   `json:"secret" yaml:"secret"`
	Byoc               bool     `json:"byoc" yaml:"byoc"`
	PublicHosts        []string `json:"public_hosts" yaml:"public-hosts"`
	Server             string   `json:"server" yaml:"server"`
	SkipSignatureCheck bool     `json:"skip_signature_check" yaml:"skip-signature-check"`
	Storages           []string `json:"storages" yaml:"storages"`
}

type ClusterGeneralConfig struct {
	PublicHost        string `json:"public_host"`
	PublicPort        uint16 `json:"public_port"`
	NoFastEnable      bool   `json:"no_fast_enable"`
	MaxReconnectCount int    `json:"max_reconnect_count"`
}

type UserItem struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

type CertificateConfig struct {
	Cert string `json:"cert" yaml:"cert"`
	Key  string `json:"key" yaml:"key"`
}

type DatabaseConfig struct {
	Driver string `json:"driver" yaml:"driver"`
	DSN    string `json:"data_source_name" yaml:"data-source-name"`
}

type HijackConfig struct {
	Enable           bool       `json:"enable" yaml:"enable"`
	EnableLocalCache bool       `json:"enable_local_cache" yaml:"enable-local-cache"`
	LocalCachePath   string     `json:"local_cache_path" yaml:"local-cache-path"`
	RequireAuth      bool       `json:"require_auth" yaml:"require-auth"`
	AuthUsers        []UserItem `json:"auth_users" yaml:"auth-users"`
}

type CacheConfig struct {
	Type string `json:"type" yaml:"type"`
	Data any    `json:"data" yaml:"data,omitempty"`

	newCache func() cache.Cache `json:"-" yaml:"-"`
}

func (c *CacheConfig) NewCache() cache.Cache {
	return c.newCache()
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
	switch c.Type {
	case "no-cache":
		c.newCache = func() cache.Cache { return cache.NoCache }
	case "memory":
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

type ServeLimitConfig struct {
	Enable     bool `json:"enable" yaml:"enable"`
	MaxConn    int  `json:"max_conn" yaml:"max-conn"`
	UploadRate int  `json:"upload_rate" yaml:"upload-rate"`
}

type GithubAPIConfig struct {
	UpdateCheckInterval utils.YAMLDuration `json:"update_check_interval" yaml:"update-check-interval"`
	Authorization       string             `json:"authorization" yaml:"authorization"`
}

type TunnelConfig struct {
	Enable      bool   `json:"enable" yaml:"enable"`
	TunnelProg  string `json:"tunnel_program" yaml:"tunnel-program"`
	OutputRegex string `json:"output_regex" yaml:"output-regex"`

	outputRegex *regexp.Regexp
	hostOut     int
	portOut     int
}

func (c *TunnelConfig) UnmarshalYAML(n *yaml.Node) (err error) {
	type T TunnelConfig
	if err = n.Decode((*T)(c)); err != nil {
		return
	}
	if !c.Enable {
		return
	}
	if c.outputRegex, err = regexp.Compile(c.OutputRegex); err != nil {
		return
	}
	c.hostOut = c.outputRegex.SubexpIndex("host")
	c.portOut = c.outputRegex.SubexpIndex("port")
	if c.hostOut <= 0 {
		return errors.New("tunneler.output-regex: missing named `(?<host>)` capture group")
	}
	if c.portOut <= 0 {
		return errors.New("tunneler.output-regex: missing named `(?<port>)` capture group")
	}
	return
}

func (c *TunnelConfig) MatchTunnelOutput(line []byte) (host, port []byte, ok bool) {
	res := c.outputRegex.FindSubmatch(line)
	if res == nil {
		return
	}
	return res[c.hostOut], res[c.portOut], true
}
