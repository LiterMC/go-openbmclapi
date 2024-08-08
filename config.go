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

	"gopkg.in/yaml.v3"

	"github.com/LiterMC/go-openbmclapi/config"
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
	if oldConfig["clusters"].(map[string]any) == nil {
		id, ok1 := oldConfig["cluster-id"].(string)
		secret, ok2 := oldConfig["cluster-secret"].(string)
		if ok1 && ok2 {
			cfg.Clusters = map[string]ClusterItem{
				"main": {
					Id:     id,
					Secret: secret,
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
		log.TrError("error.config.not.exists")
		notexists = true
	} else {
		migrateConfig(data, cfg)
		if err = cfg.UnmarshalText(data); err != nil {
			log.TrErrorf("error.config.parse.failed", err)
			os.Exit(1)
		}
		if len(cfg.Clusters) == 0 {
			cfg.Clusters = map[string]ClusterItem{
				"main": {
					Id:     "${CLUSTER_ID}",
					Secret: "${CLUSTER_SECRET}",
				},
			}
		}
		if len(cfg.Certificates) == 0 {
			cfg.Certificates = []CertificateConfig{
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
			if s.Cluster != "" && s.Cluster != "-" {
				if _, ok := cfg.Clusters[s.Cluster]; !ok {
					log.Errorf("Storage %q is trying to connect to a not exists cluster %q.", s.Id, s.Cluster)
					os.Exit(1)
				}
			}
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
		log.TrError("error.config.created")
	}
	return
}
