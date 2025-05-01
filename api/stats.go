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

package api

import (
	"strconv"
	"time"
)

type StatsManager interface {
	GetStatus() StatusData
	// returns a cluster's stat data
	// if name is empty then gets the overall access data
	GetClusterAccessStat(name string) *AccessStatData
	// returns a storage's stat data
	// if name is empty then gets the overall access data
	GetStorageAccessStat(name string) *AccessStatData
}

type StatusData struct {
	StartAt  time.Time `json:"startAt"`
	Clusters []string  `json:"clusters"`
	Storages []string  `json:"storages"`
}

type StatInstData struct {
	Hits  int32 `json:"hits"`
	Bytes int64 `json:"bytes"`
}

func (d *StatInstData) update(o *StatInstData) {
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
	StatDataHours  = [24]StatInstData
	StatDataDays   = [31]StatInstData
	StatDataMonths = [12]StatInstData
)

type AccessStatHistoryData struct {
	Hours  StatDataHours  `json:"hours"`
	Days   StatDataDays   `json:"days"`
	Months StatDataMonths `json:"months"`
}

type AccessStatData struct {
	Date statTime `json:"date"`
	AccessStatHistoryData
	Prev  AccessStatHistoryData   `json:"prev"`
	Years map[string]StatInstData `json:"years"`

	Accesses map[string]int `json:"accesses"`
}

func NewAccessStatData() *AccessStatData {
	return &AccessStatData{
		Years:    make(map[string]StatInstData, 2),
		Accesses: make(map[string]int, 5),
	}
}

func (d *AccessStatData) Clone() *AccessStatData {
	cloned := new(AccessStatData)
	*cloned = *d
	cloned.Years = make(map[string]StatInstData, len(d.Years))
	for k, v := range d.Years {
		cloned.Years[k] = v
	}
	cloned.Accesses = make(map[string]int, len(d.Accesses))
	for k, v := range d.Accesses {
		cloned.Accesses[k] = v
	}
	return cloned
}

func (d *AccessStatData) Update(newData *StatInstData) {
	now := makeStatTime(time.Now())
	if d.Date.Year != 0 {
		switch {
		case d.Date.Year != now.Year:
			iscont := now.Year == d.Date.Year+1
			isMonthCont := iscont && now.Month == 0 && d.Date.Month+1 == len(d.Months)
			var inst StatInstData
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
							d.Prev.Hours[i] = StatInstData{}
						}
					} else {
						d.Prev.Hours = StatDataHours{}
					}
					d.Hours = StatDataHours{}
					d.Prev.Days = d.Days
					for i := d.Date.Day + 1; i < len(d.Days); i++ {
						d.Prev.Days[i] = StatInstData{}
					}
				} else {
					d.Prev.Days = StatDataDays{}
				}
				d.Days = StatDataDays{}
				d.Prev.Months = d.Months
				for i := d.Date.Month + 1; i < len(d.Months); i++ {
					d.Prev.Months[i] = StatInstData{}
				}
			} else {
				d.Prev.Months = StatDataMonths{}
			}
			d.Months = StatDataMonths{}
		case d.Date.Month != now.Month:
			iscont := now.Month == d.Date.Month+1
			var inst StatInstData
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
				d.Months[i] = StatInstData{}
			}
			clear(d.Accesses)
			// update history data
			if iscont {
				if now.Day == 0 && d.Date.IsLastDay() {
					d.Prev.Hours = d.Hours
					for i := d.Date.Hour + 1; i < len(d.Hours); i++ {
						d.Prev.Hours[i] = StatInstData{}
					}
				} else {
					d.Prev.Hours = StatDataHours{}
				}
				d.Hours = StatDataHours{}
				d.Prev.Days = d.Days
				for i := d.Date.Day + 1; i < len(d.Days); i++ {
					d.Prev.Days[i] = StatInstData{}
				}
			} else {
				d.Prev.Days = StatDataDays{}
			}
			d.Days = StatDataDays{}
		case d.Date.Day != now.Day:
			var inst StatInstData
			for i := 0; i <= d.Date.Hour; i++ {
				inst.update(&d.Hours[i])
			}
			d.Days[d.Date.Day] = inst
			// clean up
			for i := d.Date.Day + 1; i < now.Day; i++ {
				d.Days[i] = StatInstData{}
			}
			// update history data
			if now.Day == d.Date.Day+1 {
				d.Prev.Hours = d.Hours
				for i := d.Date.Hour + 1; i < len(d.Hours); i++ {
					d.Prev.Hours[i] = StatInstData{}
				}
			} else {
				d.Prev.Hours = StatDataHours{}
			}
			d.Hours = StatDataHours{}
		case d.Date.Hour != now.Hour:
			// clean up
			for i := d.Date.Hour + 1; i < now.Hour; i++ {
				d.Hours[i] = StatInstData{}
			}
		}
	}

	d.Hours[now.Hour].update(newData)
	d.Date = now
}
