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

package cluster

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/internal/build"
)

const statsOverallFileName = "stat.json"

type StatManager struct {
	mux sync.RWMutex

	clusters []string
	storages []string

	Overall  *api.AccessStatData
	Clusters map[string]*api.AccessStatData
	Storages map[string]*api.AccessStatData
}

var _ json.Marshaler = (*StatManager)(nil)

func NewStatManager() *StatManager {
	return &StatManager{
		Overall:  new(api.AccessStatData),
		Clusters: make(map[string]*api.AccessStatData),
		Storages: make(map[string]*api.AccessStatData),
	}
}

func (m *StatManager) GetStatus() api.StatusData {
	m.mux.RLock()
	defer m.mux.RUnlock()

	return api.StatusData{
		StartAt:  build.StartAt,
		Clusters: m.clusters,
		Storages: m.storages,
	}
}

func (m *StatManager) AddCluster(name string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	i := sort.SearchStrings(m.clusters, name)
	if i == len(m.clusters) || m.clusters[i] != name {
		m.clusters = slices.Insert(m.clusters, i, name)
	}
}

func (m *StatManager) AddStorage(name string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	i := sort.SearchStrings(m.storages, name)
	if i == len(m.storages) || m.storages[i] != name {
		m.storages = slices.Insert(m.storages, i, name)
	}
}

func (m *StatManager) RemoveCluster(name string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	i := sort.SearchStrings(m.clusters, name)
	if i < len(m.clusters) && m.clusters[i] == name {
		m.clusters = slices.Delete(m.clusters, i, i+1)
	}
}

func (m *StatManager) RemoveStorage(name string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	i := sort.SearchStrings(m.storages, name)
	if i < len(m.storages) && m.storages[i] == name {
		m.storages = slices.Delete(m.storages, i, i+1)
	}
}

func (m *StatManager) RenameCluster(oldName, newName string) {
	if oldName == newName {
		return
	}
	m.mux.Lock()
	defer m.mux.Unlock()

	oldInd := sort.SearchStrings(m.clusters, oldName)
	if oldInd == len(m.clusters) || m.clusters[oldInd] != oldName {
		return
	}
	newInd := sort.SearchStrings(m.clusters, newName)
	if oldInd == newInd || oldInd+1 == newInd {
		m.clusters[oldInd] = newName
	} else if oldInd < newInd {
		copy(m.clusters[oldInd:], m.clusters[oldInd+1:newInd])
		m.clusters[newInd-1] = newName
	} else /*if oldInd > newInd*/ {
		copy(m.clusters[newInd+1:], m.clusters[newInd:oldInd])
		m.clusters[newInd] = newName
	}
	m.Clusters[newName] = m.Clusters[oldName]
	delete(m.Clusters, oldName)
}

func (m *StatManager) RenameStorage(oldName, newName string) {
	if oldName == newName {
		return
	}
	m.mux.Lock()
	defer m.mux.Unlock()

	oldInd := sort.SearchStrings(m.storages, oldName)
	if oldInd == len(m.storages) || m.storages[oldInd] != oldName {
		return
	}
	newInd := sort.SearchStrings(m.storages, newName)
	if oldInd == newInd || oldInd+1 == newInd {
		m.storages[oldInd] = newName
	} else if oldInd < newInd {
		copy(m.storages[oldInd:], m.storages[oldInd+1:newInd])
		m.storages[newInd-1] = newName
	} else /*if oldInd > newInd*/ {
		copy(m.storages[newInd+1:], m.storages[newInd:oldInd])
		m.storages[newInd] = newName
	}
	m.Storages[newName] = m.Storages[oldName]
	delete(m.Storages, oldName)
}

var emptyStat = api.NewAccessStatData()

func (m *StatManager) GetClusterAccessStat(name string) *api.AccessStatData {
	d := m.Clusters[name]
	if d == nil {
		return emptyStat
	}
	return d
}

func (m *StatManager) GetStorageAccessStat(name string) *api.AccessStatData {
	d := m.Storages[name]
	if d == nil {
		return emptyStat
	}
	return d
}

func (m *StatManager) AddHit(bytes int64, cluster, storage string, userAgent string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	data := &api.StatInstData{
		Hits:  1,
		Bytes: bytes,
	}
	m.Overall.Update(data)
	if userAgent != "" {
		m.Overall.Accesses[userAgent]++
	}
	if cluster != "" {
		d := m.Clusters[cluster]
		if d == nil {
			d = api.NewAccessStatData()
			m.Clusters[cluster] = d
		}
		d.Update(data)
		if userAgent != "" {
			d.Accesses[userAgent]++
		}
	}
	if storage != "" {
		d := m.Storages[storage]
		if d == nil {
			d = api.NewAccessStatData()
			m.Storages[storage] = d
		}
		d.Update(data)
		if userAgent != "" {
			d.Accesses[userAgent]++
		}
	}
}

func (m *StatManager) Load(dir string) error {
	clustersDir, storagesDir := filepath.Join(dir, "clusters"), filepath.Join(dir, "storages")

	m.mux.Lock()
	defer m.mux.Unlock()

	*m.Overall = api.AccessStatData{}
	clear(m.Clusters)
	clear(m.Storages)

	if err := loadStatData(m.Overall, filepath.Join(dir, statsOverallFileName)); err != nil {
		return err
	}
	if entries, err := os.ReadDir(clustersDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if name, ok := strings.CutSuffix(entry.Name(), ".json"); ok {
				d := new(api.AccessStatData)
				if err := loadStatData(d, filepath.Join(clustersDir, entry.Name())); err != nil {
					return err
				}
				m.Clusters[name] = d
			}
		}
	}
	if entries, err := os.ReadDir(storagesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if name, ok := strings.CutSuffix(entry.Name(), ".json"); ok {
				d := new(api.AccessStatData)
				if err := loadStatData(d, filepath.Join(storagesDir, entry.Name())); err != nil {
					return err
				}
				m.Storages[name] = d
			}
		}
	}
	return nil
}

func (m *StatManager) Save(dir string) error {
	clustersDir, storagesDir := filepath.Join(dir, "clusters"), filepath.Join(dir, "storages")

	m.mux.RLock()
	defer m.mux.RUnlock()

	if err := saveStatData(m.Overall, filepath.Join(dir, statsOverallFileName)); err != nil {
		return err
	}
	if err := os.Mkdir(clustersDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := os.Mkdir(storagesDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	for name, data := range m.Clusters {
		if err := saveStatData(data, filepath.Join(clustersDir, name+".json")); err != nil {
			return err
		}
	}
	for name, data := range m.Storages {
		if err := saveStatData(data, filepath.Join(storagesDir, name+".json")); err != nil {
			return err
		}
	}
	return nil
}

func (m *StatManager) MarshalJSON() ([]byte, error) {
	m.mux.RLock()
	defer m.mux.RUnlock()

	return json.Marshal(map[string]any{
		"overall":  m.Overall,
		"clusters": m.Clusters,
		"storages": m.Storages,
	})
}

func loadStatData(s *api.AccessStatData, name string) error {
	if err := parseFileOrOld(name, func(buf []byte) error {
		return json.Unmarshal(buf, s)
	}); err != nil {
		return err
	}

	if s.Years == nil {
		s.Years = make(map[string]api.StatInstData, 2)
	}
	if s.Accesses == nil {
		s.Accesses = make(map[string]int, 5)
	}
	return nil
}

func saveStatData(s *api.AccessStatData, name string) error {
	buf, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := writeFileWithOld(name, buf, 0644); err != nil {
		return err
	}
	return nil
}

func parseFileOrOld(path string, parser func(buf []byte) error) error {
	oldpath := path + ".old"
	buf, err := os.ReadFile(path)
	if err == nil {
		if err = parser(buf); err == nil {
			return err
		}
	}
	buf, er := os.ReadFile(oldpath)
	if er == nil {
		if er = parser(buf); er == nil {
			os.WriteFile(path, buf, 0644)
			return nil
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		if errors.Is(er, os.ErrNotExist) {
			return nil
		}
		err = er
	}
	return err
}

func writeFileWithOld(path string, buf []byte, mode os.FileMode) error {
	oldpath := path + ".old"
	if err := os.Remove(oldpath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(path, oldpath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(path, buf, mode); err != nil {
		return err
	}
	if err := os.WriteFile(oldpath, buf, mode); err != nil {
		return err
	}
	return nil
}
