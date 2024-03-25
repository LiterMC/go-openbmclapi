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

package limited

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LiterMC/go-openbmclapi/utils"
)

type RateLimit struct {
	PerMin  int64 `json:"per-minute" yaml:"per-minute"`
	PerHour int64 `json:"per-hour" yaml:"per-hour"`
}

type limitSet struct {
	Limit RateLimit

	mux        sync.RWMutex
	accessMin  map[string]*atomic.Int64
	accessHour map[string]*atomic.Int64
}

func makeLimitSet() limitSet {
	return limitSet{
		accessMin:  make(map[string]*atomic.Int64),
		accessHour: make(map[string]*atomic.Int64),
	}
}

func (s *limitSet) try(id string) (leftHour, leftMin int64, ok bool) {
	if s.Limit.PerHour == 0 && s.Limit.PerMin == 0 {
		ok = true
		return
	}
	s.mux.RLock()
	hour, ok := s.accessHour[id]
	s.mux.RUnlock()
	if !ok {
		s.mux.Lock()
		if hour, ok = s.accessHour[id]; !ok {
			hour = new(atomic.Int64)
			s.accessHour[id] = hour
		}
		s.mux.RUnlock()
	}
	leftHour = s.Limit.PerHour - hour.Add(1)
	if leftHour < 0 {
		hour.Add(-1)
		leftHour = 0
		return
	}

	if s.Limit.PerMin == 0 {
		ok = true
		return
	}
	s.mux.RLock()
	min, ok := s.accessMin[id]
	s.mux.RUnlock()
	if !ok {
		s.mux.Lock()
		if min, ok = s.accessMin[id]; !ok {
			min = new(atomic.Int64)
			s.accessMin[id] = min
		}
		s.mux.RUnlock()
	}
	leftMin = s.Limit.PerMin - min.Add(1)
	if leftMin < 0 {
		min.Add(-1)
		leftMin = 0
		return
	}
	ok = true
	return
}

func (s *limitSet) clean(hour bool) {
	s.mux.Lock()
	defer s.mux.Unlock()
	clear(s.accessMin)
	if hour {
		clear(s.accessHour)
	}
}

type APIRateMiddleWare struct {
	realIPContextKey, loggedContextKey any

	annoySet  limitSet
	loggedSet limitSet

	cleanTimer *time.Timer
	startAt    time.Time
}

func NewAPIRateMiddleWare(realIPContextKey, loggedContextKey any) (a *APIRateMiddleWare) {
	a = &APIRateMiddleWare{
		loggedContextKey: loggedContextKey,
		annoySet:         makeLimitSet(),
		loggedSet:        makeLimitSet(),
		cleanTimer:       time.NewTimer(time.Minute),
		startAt:          time.Now(),
	}
	go func() {
		count := 0
		for range a.cleanTimer.C {
			count++
			ishour := count > 60
			if ishour {
				count = 0
			}
			a.clean(ishour)
		}
	}()
	return
}

var _ utils.MiddleWare = (*APIRateMiddleWare)(nil)

const (
	RateLimitOverrideContextKey = "go-openbmclapi.limited.rate.api.override"
)

func (a *APIRateMiddleWare) AnonymousRateLimit() RateLimit {
	return a.annoySet.Limit
}

func (a *APIRateMiddleWare) SetAnonymousRateLimit(v RateLimit) {
	a.annoySet.Limit = v
}

func (a *APIRateMiddleWare) LoggedRateLimit() RateLimit {
	return a.loggedSet.Limit
}

func (a *APIRateMiddleWare) SetLoggedRateLimit(v RateLimit) {
	a.loggedSet.Limit = v
}

func (a *APIRateMiddleWare) Destroy() {
	a.cleanTimer.Stop()
}

func (a *APIRateMiddleWare) clean(ishour bool) {
	a.loggedSet.clean(ishour)
	a.annoySet.clean(ishour)
}

func (a *APIRateMiddleWare) ServeMiddle(rw http.ResponseWriter, req *http.Request, next http.Handler) {
	var set *limitSet
	id, ok := req.Context().Value(a.loggedContextKey).(string)
	if ok {
		set = &a.loggedSet
	} else {
		id, _ = req.Context().Value(a.realIPContextKey).(string)
		if id == "" {
			id, _, _ = net.SplitHostPort(req.RemoteAddr)
		}
		set = &a.annoySet
	}
	hourLeft, minLeft, pass := set.try(id)
	now := time.Now()
	var retryAfter int
	if hourLeft == 0 {
		retryAfter = 3600 - (int)(now.Sub(a.startAt)/time.Second%3600)
	} else {
		retryAfter = 60 - (int)(now.Sub(a.startAt)/time.Second%60)
	}
	resetAfter := now.Add((time.Duration)(retryAfter) * time.Second).Unix()
	rw.Header().Set("X-Ratelimit-Limit-Minute", strconv.FormatInt(set.Limit.PerMin, 10))
	rw.Header().Set("X-Ratelimit-Limit-Hour", strconv.FormatInt(set.Limit.PerHour, 10))
	rw.Header().Set("X-Ratelimit-Remaining-Minute", strconv.FormatInt(minLeft, 10))
	rw.Header().Set("X-Ratelimit-Remaining-Hour", strconv.FormatInt(hourLeft, 10))
	rw.Header().Set("X-Ratelimit-Reset-After", strconv.FormatInt(resetAfter, 10))
	if !pass {
		rw.Header().Set("Retry-After", strconv.Itoa(retryAfter+1))
		rw.WriteHeader(http.StatusTooManyRequests)
		return
	}
	srw := utils.WrapAsStatusResponseWriter(rw)
	next.ServeHTTP(srw, req)
	if srw.Status/100 == 3 {
		// TODO: release limit
		return
	}
}

func (a *APIRateMiddleWare) WrapHandler(next http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		a.ServeMiddle(rw, req, next)
	}
}
