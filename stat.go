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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

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
	statDataHours  [24]statInstData
	statDataDays   [31]statInstData
	statDataMonths [12]statInstData
)

type statHistoryData struct {
	Hours  statDataHours  `json:"hours"`
	Days   statDataDays   `json:"days"`
	Months statDataMonths `json:"months"`
}

type statData struct {
	Date statTime `json:"date"`
	statHistoryData
	Prev  statHistoryData         `json:"prev"`
	Years map[string]statInstData `json:"years"`

	Accesses map[string]int `json:"accesses"`
}

func (d *statData) update(newData *statInstData) {
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

type Stats struct {
	mux sync.RWMutex
	statData
}

const statsFileName = "stat.json"

func (s *Stats) MarshalJSON() ([]byte, error) {
	s.mux.RLock()
	defer s.mux.RUnlock()

	return json.Marshal(&s.statData)
}

func (s *Stats) Load(dir string) (err error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if err = parseFileOrOld(filepath.Join(dir, statsFileName), func(buf []byte) error {
		return json.Unmarshal(buf, &s.statData)
	}); err != nil {
		return
	}

	if s.Years == nil {
		s.Years = make(map[string]statInstData, 2)
	}
	if s.Accesses == nil {
		s.Accesses = make(map[string]int, 5)
	}
	return
}

// Save
func (s *Stats) Save(dir string) (err error) {
	s.mux.RLock()
	defer s.mux.RUnlock()

	buf, err := json.Marshal(&s.statData)
	if err != nil {
		return
	}

	if err = writeFileWithOld(filepath.Join(dir, statsFileName), buf, 0644); err != nil {
		return
	}
	return
}

func (s *Stats) AddHits(hits int32, bytes int64) {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.update(&statInstData{
		Hits:  hits,
		Bytes: bytes,
	})
}

func parseFileOrOld(path string, parser func(buf []byte) error) (err error) {
	oldpath := path + ".old"
	buf, err := os.ReadFile(path)
	if err == nil {
		if err = parser(buf); err == nil {
			return
		}
	}
	buf, er := os.ReadFile(oldpath)
	if er == nil {
		if er = parser(buf); er == nil {
			return
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		if errors.Is(er, os.ErrNotExist) {
			return nil
		}
		err = er
	}
	return
}

func writeFileWithOld(path string, buf []byte, mode os.FileMode) (err error) {
	oldpath := path + ".old"
	if err = os.Remove(oldpath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	if err = os.Rename(path, oldpath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	if err = os.WriteFile(path, buf, mode); err != nil {
		return
	}
	if err = os.WriteFile(oldpath, buf, mode); err != nil {
		return
	}
	return
}
