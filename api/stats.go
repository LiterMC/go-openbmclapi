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
	"time"
)

type StatsManager interface {
	GetStatus() StatusData
	// if name is empty then gets the overall access data
	GetAccessStat(name string) *AccessStatData
}

type StatusData struct {
	StartAt  time.Time `json:"startAt"`
	Clusters []string  `json:"clusters"`
	Storages []string  `json:"storages"`
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

type accessStatHistoryData struct {
	Hours  statDataHours  `json:"hours"`
	Days   statDataDays   `json:"days"`
	Months statDataMonths `json:"months"`
}

type AccessStatData struct {
	Date statTime `json:"date"`
	accessStatHistoryData
	Prev  accessStatHistoryData   `json:"prev"`
	Years map[string]statInstData `json:"years"`

	Accesses map[string]int `json:"accesses"`
}
