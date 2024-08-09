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
	"strconv"
	"strings"
	"sync"
	"time"
)

const statsOverallFileName = "stat.json"

type StatManager struct {
	mux sync.RWMutex

	Overall  *StatData
	Clusters map[string]*StatData
	Storages map[string]*StatData
}

var _ json.Marshaler = (*StatManager)(nil)

func NewStatManager() *StatManager {
	return &StatManager{
		Overall:  new(StatData),
		Clusters: make(map[string]*StatData),
		Storages: make(map[string]*StatData),
	}
}

func (m *StatManager) AddHit(bytes int64, cluster, storage string) {
	m.mux.Lock()
	defer m.mux.Unlock()

	data := &statInstData{
		Hits:  1,
		Bytes: bytes,
	}
	m.Overall.update(data)
	if cluster != "" {
		d := m.Clusters[cluster]
		if d == nil {
			d = NewStatData()
			m.Clusters[cluster] = d
		}
		d.update(data)
	}
	if storage != "" {
		d := m.Storages[storage]
		if d == nil {
			d = NewStatData()
			m.Storages[storage] = d
		}
		d.update(data)
	}
}

func (m *StatManager) Load(dir string) error {
	clustersDir, storagesDir := filepath.Join(dir, "clusters"), filepath.Join(dir, "storages")

	m.mux.Lock()
	defer m.mux.Unlock()

	*m.Overall = StatData{}
	clear(m.Clusters)
	clear(m.Storages)

	if err := m.Overall.load(filepath.Join(dir, statsOverallFileName)); err != nil {
		return err
	}
	if entries, err := os.ReadDir(clustersDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if name, ok := strings.CutSuffix(entry.Name(), ".json"); ok {
				d := new(StatData)
				if err := d.load(filepath.Join(clustersDir, entry.Name())); err != nil {
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
				d := new(StatData)
				if err := d.load(filepath.Join(storagesDir, entry.Name())); err != nil {
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

	if err := m.Overall.save(filepath.Join(dir, statsOverallFileName)); err != nil {
		return err
	}
	if err := os.Mkdir(clustersDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := os.Mkdir(storagesDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	for name, data := range m.Clusters {
		if err := data.save(filepath.Join(clustersDir, name+".json")); err != nil {
			return err
		}
	}
	for name, data := range m.Storages {
		if err := data.save(filepath.Join(storagesDir, name+".json")); err != nil {
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

type statInstData struct {
	Hits  int32 `json:"hits"`
	Bytes int64 `json:"bytes"`
}

func (d *statInstData) update(o *statInstData) {
	d.Hits += o.Hits
	d.Bytes += o.Bytes
}

// statTime always save a UTC time
type statTime struct {
	Hour  int `json:"hour"`
	Day   int `json:"day"`
	Month int `json:"month"`
	Year  int `json:"year"`
}

func makeStatTime(t time.Time) (st statTime) {
	t = t.UTC()
	st.Hour = t.Hour()
	y, m, d := t.Date()
	st.Day = d - 1
	st.Month = (int)(m) - 1
	st.Year = y
	return
}

func (t statTime) IsLastDay() bool {
	return time.Date(t.Year, (time.Month)(t.Month+1), t.Day+1+1, 0, 0, 0, 0, time.UTC).Day() == 1
}

type (
	statDataHours  = [24]statInstData
	statDataDays   = [31]statInstData
	statDataMonths = [12]statInstData
)

type statHistoryData struct {
	Hours  statDataHours  `json:"hours"`
	Days   statDataDays   `json:"days"`
	Months statDataMonths `json:"months"`
}

type StatData struct {
	Date statTime `json:"date"`
	statHistoryData
	Prev  statHistoryData         `json:"prev"`
	Years map[string]statInstData `json:"years"`

	Accesses map[string]int `json:"accesses"`
}

func NewStatData() *StatData {
	return &StatData{
		Years:    make(map[string]statInstData, 2),
		Accesses: make(map[string]int, 5),
	}
}
func (d *StatData) Clone() *StatData {
	cloned := new(StatData)
	*cloned = *d
	cloned.Years = make(map[string]statInstData, len(d.Years))
	for k, v := range d.Years {
		cloned.Years[k] = v
	}
	cloned.Accesses = make(map[string]int, len(d.Accesses))
	for k, v := range d.Accesses {
		cloned.Accesses[k] = v
	}
	return cloned
}

func (d *StatData) update(newData *statInstData) {
	now := makeStatTime(time.Now())
	if d.Date.Year != 0 {
		switch {
		case d.Date.Year != now.Year:
			iscont := now.Year == d.Date.Year+1
			isMonthCont := iscont && now.Month == 0 && d.Date.Month+1 == len(d.Months)
			var inst statInstData
			for i := 0; i < d.Date.Month; i++ {
				inst.update(&d.Months[i])
			}
			if iscont {
				for i := 0; i <= d.Date.Day; i++ {
					inst.update(&d.Days[i])
				}
				if isMonthCont {
					for i := 0; i <= d.Date.Hour; i++ {
						inst.update(&d.Hours[i])
					}
				}
			}
			d.Years[strconv.Itoa(d.Date.Year)] = inst
			// update history data
			if iscont {
				if isMonthCont {
					if now.Day == 0 && d.Date.IsLastDay() {
						d.Prev.Hours = d.Hours
						for i := d.Date.Hour + 1; i < len(d.Hours); i++ {
							d.Prev.Hours[i] = statInstData{}
						}
					} else {
						d.Prev.Hours = statDataHours{}
					}
					d.Hours = statDataHours{}
					d.Prev.Days = d.Days
					for i := d.Date.Day + 1; i < len(d.Days); i++ {
						d.Prev.Days[i] = statInstData{}
					}
				} else {
					d.Prev.Days = statDataDays{}
				}
				d.Days = statDataDays{}
				d.Prev.Months = d.Months
				for i := d.Date.Month + 1; i < len(d.Months); i++ {
					d.Prev.Months[i] = statInstData{}
				}
			} else {
				d.Prev.Months = statDataMonths{}
			}
			d.Months = statDataMonths{}
		case d.Date.Month != now.Month:
			iscont := now.Month == d.Date.Month+1
			var inst statInstData
			for i := 0; i < d.Date.Day; i++ {
				inst.update(&d.Days[i])
			}
			if iscont {
				for i := 0; i <= d.Date.Hour; i++ {
					inst.update(&d.Hours[i])
				}
			}
			d.Months[d.Date.Month] = inst
			// clean up
			for i := d.Date.Month + 1; i < now.Month; i++ {
				d.Months[i] = statInstData{}
			}
			clear(d.Accesses)
			// update history data
			if iscont {
				if now.Day == 0 && d.Date.IsLastDay() {
					d.Prev.Hours = d.Hours
					for i := d.Date.Hour + 1; i < len(d.Hours); i++ {
						d.Prev.Hours[i] = statInstData{}
					}
				} else {
					d.Prev.Hours = statDataHours{}
				}
				d.Hours = statDataHours{}
				d.Prev.Days = d.Days
				for i := d.Date.Day + 1; i < len(d.Days); i++ {
					d.Prev.Days[i] = statInstData{}
				}
			} else {
				d.Prev.Days = statDataDays{}
			}
			d.Days = statDataDays{}
		case d.Date.Day != now.Day:
			var inst statInstData
			for i := 0; i <= d.Date.Hour; i++ {
				inst.update(&d.Hours[i])
			}
			d.Days[d.Date.Day] = inst
			// clean up
			for i := d.Date.Day + 1; i < now.Day; i++ {
				d.Days[i] = statInstData{}
			}
			// update history data
			if now.Day == d.Date.Day+1 {
				d.Prev.Hours = d.Hours
				for i := d.Date.Hour + 1; i < len(d.Hours); i++ {
					d.Prev.Hours[i] = statInstData{}
				}
			} else {
				d.Prev.Hours = statDataHours{}
			}
			d.Hours = statDataHours{}
		case d.Date.Hour != now.Hour:
			// clean up
			for i := d.Date.Hour + 1; i < now.Hour; i++ {
				d.Hours[i] = statInstData{}
			}
		}
	}

	d.Hours[now.Hour].update(newData)
	d.Date = now
}

func (s *StatData) load(name string) error {
	if err := parseFileOrOld(name, func(buf []byte) error {
		return json.Unmarshal(buf, s)
	}); err != nil {
		return err
	}

	if s.Years == nil {
		s.Years = make(map[string]statInstData, 2)
	}
	if s.Accesses == nil {
		s.Accesses = make(map[string]int, 5)
	}
	return nil
}

func (s *StatData) save(name string) error {
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
