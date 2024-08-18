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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/cluster"
	"github.com/LiterMC/go-openbmclapi/config"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/utils"
)

func migrateConfig(data []byte, cfg *config.Config) {
	var oldConfig map[string]any
	if err := yaml.Unmarshal(data, &oldConfig); err != nil {
		return
	}

	if v, ok := oldConfig["debug"].(bool); ok {
		cfg.Advanced.DebugLog = v
	}
	if v, ok := oldConfig["no-heavy-check"].(bool); ok {
		cfg.Advanced.NoHeavyCheck = v
	}
	if v, ok := oldConfig["keepalive-timeout"].(int); ok {
		cfg.Advanced.KeepaliveTimeout = v
	}
	if oldConfig["clusters"] == nil {
		id, ok1 := oldConfig["cluster-id"].(string)
		secret, ok2 := oldConfig["cluster-secret"].(string)
		publicHost, ok3 := oldConfig["public-host"].(string)
		if ok1 && ok2 && ok3 {
			cfg.Clusters = map[string]config.ClusterOptions{
				"main": {
					Id:          id,
					Secret:      secret,
					PublicHosts: []string{publicHost},
				},
			}
		}
	}
}

func readAndRewriteConfig() (cfg *config.Config, err error) {
	const configPath = "config.yaml"

	cfg = config.NewDefaultConfig()
	data, err := os.ReadFile(configPath)
	notexists := false
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.TrErrorf("error.config.read.failed", err)
			os.Exit(1)
		}
		log.TrErrorf("error.config.not.exists")
		notexists = true
	} else {
		migrateConfig(data, cfg)
		if err = cfg.UnmarshalText(data); err != nil {
			log.TrErrorf("error.config.parse.failed", err)
			os.Exit(1)
		}
		if len(cfg.Clusters) == 0 {
			cfg.Clusters = map[string]config.ClusterOptions{
				"main": {
					Id:                 "${CLUSTER_ID}",
					Secret:             "${CLUSTER_SECRET}",
					PublicHosts:        []string{},
					Server:             cluster.DefaultBMCLAPIServer,
					SkipSignatureCheck: false,
				},
			}
		}
		if len(cfg.Certificates) == 0 {
			cfg.Certificates = []config.CertificateConfig{
				{
					Cert: "/path/to/cert.pem",
					Key:  "/path/to/key.pem",
				},
			}
		}
		if len(cfg.Storages) == 0 {
			cfg.Storages = []storage.StorageOption{
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
		if len(cfg.WebdavUsers) == 0 {
			cfg.WebdavUsers["example-user"] = &storage.WebDavUser{
				EndPoint: "https://webdav.example.com/path/to/endpoint/",
				Username: "example-username",
				Password: "example-password",
			}
		}
		ids := make(map[string]int, len(cfg.Storages))
		for i, s := range cfg.Storages {
			if s.Id == "" {
				s.Id = fmt.Sprintf("storage-%d", i)
				cfg.Storages[i].Id = s.Id
			}
			if j, ok := ids[s.Id]; ok {
				log.Errorf("Duplicated storage id %q at [%d] and [%d], please edit the config.", s.Id, i, j)
				os.Exit(1)
			}
			ids[s.Id] = i
		}
	}

	for _, so := range cfg.Storages {
		switch opt := so.Data.(type) {
		case *storage.WebDavStorageOption:
			if alias := opt.Alias; alias != "" {
				user, ok := cfg.WebdavUsers[alias]
				if !ok {
					log.TrErrorf("error.config.alias.user.not.exists", alias)
					os.Exit(1)
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
	if err = encoder.Encode(cfg); err != nil {
		log.TrErrorf("error.config.encode.failed", err)
		os.Exit(1)
	}
	if err = os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		log.TrErrorf("error.config.write.failed", err)
		os.Exit(1)
	}
	if notexists {
		log.TrErrorf("error.config.created")
		return nil, errors.New("Please edit the config before continue!")
	}
	return
}

type ConfigHandler struct {
	mux sync.RWMutex
	r   *Runner

	updateProcess []func(context.Context) error
}

var _ api.ConfigHandler = (*ConfigHandler)(nil)

func (c *ConfigHandler) update(newConfig *config.Config) error {
	r := c.r
	oldConfig := r.Config
	c.updateProcess = c.updateProcess[:0]

	if newConfig.LogSlots != oldConfig.LogSlots || newConfig.NoAccessLog != oldConfig.NoAccessLog || newConfig.AccessLogSlots != oldConfig.AccessLogSlots || newConfig.Advanced.DebugLog != oldConfig.Advanced.DebugLog {
		c.updateProcess = append(c.updateProcess, r.SetupLogger)
	}
	if newConfig.Host != oldConfig.Host || newConfig.Port != oldConfig.Port {
		c.updateProcess = append(c.updateProcess, r.StartServer)
	}
	if newConfig.PublicHost != oldConfig.PublicHost || newConfig.PublicPort != oldConfig.PublicPort || newConfig.Advanced.NoFastEnable != oldConfig.Advanced.NoFastEnable || newConfig.MaxReconnectCount != oldConfig.MaxReconnectCount {
		c.updateProcess = append(c.updateProcess, r.updateClustersWithGeneralConfig)
	}
	if newConfig.RateLimit != oldConfig.RateLimit {
		c.updateProcess = append(c.updateProcess, r.updateRateLimit)
	}
	if newConfig.Notification != oldConfig.Notification {
		// c.updateProcess = append(c.updateProcess, )
	}

	r.Config = newConfig
	r.publicHost = r.Config.PublicHost
	r.publicPort = r.Config.PublicPort
	return nil
}

func (c *ConfigHandler) doUpdateProcesses(ctx context.Context) error {
	for _, proc := range c.updateProcess {
		if err := proc(ctx); err != nil {
			return err
		}
	}
	c.updateProcess = c.updateProcess[:0]
	return nil
}

func (c *ConfigHandler) MarshalJSON() ([]byte, error) {
	return c.r.Config.MarshalJSON()
}

func (c *ConfigHandler) UnmarshalJSON(data []byte) error {
	c2 := c.r.Config.Clone()
	if err := c2.UnmarshalJSON(data); err != nil {
		return err
	}
	c.update(c2)
	return nil
}

func (c *ConfigHandler) UnmarshalYAML(data []byte) error {
	c2 := c.r.Config.Clone()
	if err := c2.UnmarshalText(data); err != nil {
		return err
	}
	c.update(c2)
	return nil
}

func (c *ConfigHandler) MarshalJSONPath(path string) ([]byte, error) {
	names := strings.Split(path, ".")
	data, err := c.r.Config.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	accessed := ""
	var x any = m
	for _, n := range names {
		mc, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Unexpected type %T on path %q, expect map[string]any", x, accessed)
		}
		accessed += n + "."
		x = mc[n]
	}
	return json.Marshal(x)
}

func (c *ConfigHandler) UnmarshalJSONPath(path string, data []byte) error {
	names := strings.Split(path, ".")
	var d any
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	accessed := ""
	var m map[string]any
	{
		b, err := c.MarshalJSON()
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
	}
	x := m
	for _, p := range names[:len(names)-1] {
		accessed += p + "."
		var ok bool
		x, ok = x[p].(map[string]any)
		if !ok {
			return fmt.Errorf("Unexpected type %T on path %q, expect map[string]any", x, accessed)
		}
	}
	x[names[len(names)-1]] = d
	dt, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return c.UnmarshalJSON(dt)
}

func (c *ConfigHandler) Fingerprint() string {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return c.fingerprintLocked()
}

func (c *ConfigHandler) fingerprintLocked() string {
	data, err := c.MarshalJSON()
	if err != nil {
		log.Panicf("ConfigHandler.Fingerprint: MarshalJSON: %v", err)
	}
	return utils.BytesAsSha256(data)
}

func (c *ConfigHandler) DoLockedAction(fingerprint string, callback func(api.ConfigHandler) error) error {
	if c.fingerprintLocked() != fingerprint {
		return api.ErrPreconditionFailed
	}
	return callback(c)
}
