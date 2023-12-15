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

func (d *statInstData) update(hits int32, bytes int64) {
	d.Hits += hits
	d.Bytes += bytes
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

type statHistoryData struct {
	Hours  [24]statInstData `json:"hours"`
	Days   [31]statInstData `json:"days"`
	Months [12]statInstData `json:"months"`
}

type statData struct {
	Date statTime `json:"date"`
	statHistoryData
	Prev  statHistoryData         `json:"prev"`
	Years map[string]statInstData `json:"years"`
}

func (d *statData) update(hits int32, bytes int64) {
	now := makeStatTime(time.Now())
	if d.Date.Year != 0 {
		switch {
		case d.Date.Year != now.Year:
			iscont := now.Year == d.Date.Year+1
			isMonthCont := iscont && now.Month == 0 && d.Date.Month+1 == len(d.Months)
			var inst statInstData
			for i := 0; i < d.Date.Month; i++ {
				n := d.Months[i]
				inst.update(n.Hits, n.Bytes)
			}
			if iscont {
				for i := 0; i <= d.Date.Day; i++ {
					n := d.Days[i]
					inst.update(n.Hits, n.Bytes)
				}
				if isMonthCont {
					for i := 0; i <= d.Date.Hour; i++ {
						n := d.Hours[i]
						inst.update(n.Hits, n.Bytes)
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
						d.Prev.Hours = [len(d.Hours)]statInstData{}
					}
					d.Prev.Days = d.Days
					for i := d.Date.Day + 1; i < len(d.Days); i++ {
						d.Prev.Days[i] = statInstData{}
					}
				} else {
					d.Prev.Days = [len(d.Days)]statInstData{}
				}
				d.Prev.Months = d.Months
				for i := d.Date.Month + 1; i < len(d.Months); i++ {
					d.Prev.Months[i] = statInstData{}
				}
			} else {
				d.Prev.Months = [len(d.Months)]statInstData{}
			}
			d.Months = [len(d.Months)]statInstData{}
		case d.Date.Month != now.Month:
			iscont := now.Month == d.Date.Month+1
			var inst statInstData
			for i := 0; i < d.Date.Day; i++ {
				n := d.Days[i]
				inst.update(n.Hits, n.Bytes)
			}
			if iscont {
				for i := 0; i <= d.Date.Hour; i++ {
					n := d.Hours[i]
					inst.update(n.Hits, n.Bytes)
				}
			}
			d.Months[d.Date.Month] = inst
			// clean up
			for i := d.Date.Month + 1; i < now.Month; i++ {
				d.Months[i] = statInstData{}
			}
			// update history data
			if iscont {
				if now.Day == 0 && d.Date.IsLastDay() {
					d.Prev.Hours = d.Hours
					for i := d.Date.Hour + 1; i < len(d.Hours); i++ {
						d.Prev.Hours[i] = statInstData{}
					}
				} else {
					d.Prev.Hours = [len(d.Hours)]statInstData{}
				}
				d.Prev.Days = d.Days
				for i := d.Date.Day + 1; i < len(d.Days); i++ {
					d.Prev.Days[i] = statInstData{}
				}
			} else {
				d.Prev.Days = [len(d.Days)]statInstData{}
			}
			d.Days = [len(d.Days)]statInstData{}
		case d.Date.Day != now.Day:
			var inst statInstData
			for i := 0; i <= d.Date.Hour; i++ {
				n := d.Hours[i]
				inst.update(n.Hits, n.Bytes)
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
				d.Prev.Hours = [24]statInstData{}
			}
			d.Hours = [24]statInstData{}
		case d.Date.Hour != now.Hour:
			// clean up
			for i := d.Date.Hour + 1; i < now.Hour; i++ {
				d.Hours[i] = statInstData{}
			}
		}
	}

	d.Hours[now.Hour].update(hits, bytes)
	d.Date = now
}

type Stats struct {
	mux sync.RWMutex
	statData
}

const statsFileName = "stat.json"

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
	return
}

// Save
func (s *Stats) Save(dir string) (err error) {
	s.mux.Lock()
	defer s.mux.Unlock()

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

	s.update(hits, bytes)
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
