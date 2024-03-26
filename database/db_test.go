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

package database_test

import (
	"encoding/json"
	"time"

	. "github.com/LiterMC/go-openbmclapi/database"
	"testing"
)

func TestUnmarshalNotificationScopes(t *testing.T) {
	data := []struct {
		S string
		V NotificationScopes
	}{
		{`[]`, NotificationScopes{}},
		{`["enabled"]`, NotificationScopes{Enabled: true}},
		{`["enabled", "enabled"]`, NotificationScopes{Enabled: true}},
		{`["enabled", "syncdone"]`, NotificationScopes{Enabled: true, SyncDone: true}},
		{`{}`, NotificationScopes{}},
		{`{"enabled": true}`, NotificationScopes{Enabled: true}},
		{`{"enabled": false}`, NotificationScopes{Enabled: false}},
		{`{"enabled": true, "syncdone": true}`, NotificationScopes{Enabled: true, SyncDone: true}},
	}
	for i, d := range data {
		var v NotificationScopes
		if e := json.Unmarshal(([]byte)(d.S), &v); e != nil {
			t.Errorf("Cannot parse %q: %v", d.S, e)
			continue
		}
		if v != d.V {
			t.Errorf("Incorrect return value at %d (%s), expect %#v, got %#v", i, d.S, d.V, v)
		}
	}
}

var TwoFirst = time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)
var TwoSecond = time.Date(2000, 01, 02, 0, 0, 0, 0, time.UTC)

func TestScheduleReadySince(t *testing.T) {
	data := []struct {
		S string
		L time.Time
		N time.Time
		V bool
	}{
		{"00:00", TwoFirst, TwoFirst, false},
		{"00:00", TwoFirst, TwoFirst.Add(time.Hour * 3), false},
		{"00:00", TwoFirst, TwoSecond, true},
		{"00:00", TwoFirst.Add(time.Minute * 59), TwoSecond, true},
		{"00:00", TwoFirst.Add(time.Hour * 12), TwoSecond, true},
		{"00:00", TwoFirst.Add(time.Hour*12 + 1), TwoSecond, false},
		{"12:00", TwoFirst, TwoSecond.Add(time.Hour*12 - 1), true},
		{"12:00", TwoFirst, TwoSecond.Add(time.Hour * 12), true},
		{"12:00", TwoFirst, TwoSecond.Add(time.Hour * 23), true},
		{"12:00", TwoFirst, TwoSecond.Add(time.Hour * 24), true},
		{"12:00", TwoFirst, TwoSecond.Add(time.Hour * 25), true},
		{"12:00", TwoFirst, TwoFirst.Add(time.Hour * 15), false},
		{"12:00", TwoFirst, TwoFirst.Add(time.Hour*15 - 1), true},
		{"23:59", TwoFirst, TwoSecond, true},
		{"00:00", time.Time{}, TwoFirst, true},
		{"00:00", time.Time{}, TwoFirst.Add(time.Hour), true},
		{"00:00", time.Time{}, TwoFirst.Add(time.Hour * 3), false},
		{"12:00", time.Time{}, TwoSecond.Add(-time.Hour + 1), false},
	}
	for i, v := range data {
		var s Schedule
		if e := s.UnmarshalText(([]byte)(v.S)); e != nil {
			t.Fatalf("Cannot parse %q: %v", v.S, e)
		}
		r := s.ReadySince(v.L, v.N)
		if r != v.V {
			t.Errorf("Incorrect return value at %d (%s), expect %v, got %v", i, v.S, v.V, r)
		}
	}
}
