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
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/LiterMC/go-openbmclapi/cache"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type ClusterOptions struct {
	Id                 string   `json:"id" yaml:"id"`
	Secret             string   `json:"secret" yaml:"secret"`
	Byoc               bool     `json:"byoc"`
	PublicHosts        []string `json:"public-hosts" yaml:"public-hosts"`
	Server             string   `json:"server" yaml:"server"`
	SkipSignatureCheck bool     `json:"skip-signature-check" yaml:"skip-signature-check"`
	Storages           []string `json:"storages" yaml:"storages"`
}

type ClusterGeneralConfig struct {
	PublicHost        string `json:"public-host"`
	PublicPort        uint16 `json:"public-port"`
	NoFastEnable      bool   `json:"no-fast-enable"`
	MaxReconnectCount int    `json:"max-reconnect-count"`
}

type UserItem struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type CertificateConfig struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"data-source-name"`
}

type HijackConfig struct {
	Enable           bool       `yaml:"enable"`
	EnableLocalCache bool       `yaml:"enable-local-cache"`
	LocalCachePath   string     `yaml:"local-cache-path"`
	RequireAuth      bool       `yaml:"require-auth"`
	AuthUsers        []UserItem `yaml:"auth-users"`
}

type CacheConfig struct {
	Type string `yaml:"type"`
	Data any    `yaml:"data,omitempty"`

	newCache func() cache.Cache `yaml:"-"`
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

type ServeLimitConfig struct {
	Enable     bool `yaml:"enable"`
	MaxConn    int  `yaml:"max-conn"`
	UploadRate int  `yaml:"upload-rate"`
}

type GithubAPIConfig struct {
	UpdateCheckInterval utils.YAMLDuration `yaml:"update-check-interval"`
	Authorization       string             `yaml:"authorization"`
}

type TunnelConfig struct {
	Enable        bool   `yaml:"enable"`
	TunnelProg    string `yaml:"tunnel-program"`
	OutputRegex   string `yaml:"output-regex"`
	TunnelTimeout int    `yaml:"tunnel-timeout"`

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
