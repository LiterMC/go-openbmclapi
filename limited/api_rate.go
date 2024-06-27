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
	"context"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type RateLimit struct {
	PerMin  int64 `json:"per-minute" yaml:"per-minute"`
	PerHour int64 `json:"per-hour" yaml:"per-hour"`
}

type limitSet struct {
	Limit RateLimit

	mux        sync.RWMutex
	cleanCount int // min clean mask: 0xffff; hour clean mask: 0xff0000
	accessMin  map[string]*atomic.Int64
	accessHour map[string]*atomic.Int64
}

func makeLimitSet() limitSet {
	return limitSet{
		cleanCount: 1,
		accessMin:  make(map[string]*atomic.Int64),
		accessHour: make(map[string]*atomic.Int64),
	}
}

func (s *limitSet) try(id string) (leftHour, leftMin int64, cleanId int) {
	checkHour, checkMin := s.Limit.PerHour > 0, s.Limit.PerMin > 0
	if !checkHour {
		leftHour = -1
	}
	if !checkMin {
		leftMin = -1
	}
	if !checkHour && !checkMin {
		cleanId = -1
		return
	}

	var (
		hour, min *atomic.Int64
		ok1, ok2  bool
	)

	s.mux.RLock()
	cleanId = s.cleanCount
	if checkHour {
		hour, ok1 = s.accessHour[id]
	}
	if checkMin {
		min, ok2 = s.accessMin[id]
	}
	s.mux.RUnlock()

	if checkHour {
		if !ok1 {
			s.mux.Lock()
			cleanId = s.cleanCount
			if hour, ok1 = s.accessHour[id]; !ok1 {
				hour = new(atomic.Int64)
				s.accessHour[id] = hour
			}
			if checkMin {
				if min, ok2 = s.accessMin[id]; !ok2 {
					min = new(atomic.Int64)
					s.accessMin[id] = min
					ok2 = true
				}
			}
			s.mux.Unlock()
		}
		leftHour = s.Limit.PerHour - hour.Add(1)
		if leftHour < 0 {
			hour.Add(-1)
			leftHour = 0
			cleanId = 0
			return
		}
	}

	if checkMin {
		if !ok2 {
			s.mux.Lock()
			cleanId = s.cleanCount
			if min, ok2 = s.accessMin[id]; !ok2 {
				min = new(atomic.Int64)
				s.accessMin[id] = min
			}
			s.mux.Unlock()
		}
		leftMin = s.Limit.PerMin - min.Add(1)
		if leftMin < 0 {
			hour.Add(-1)
			min.Add(-1)
			leftMin = 0
			cleanId = 0
			return
		}
	}
	return
}

func (s *limitSet) release(id string, cleanId int) {
	if cleanId <= 0 {
		return
	}
	checkHour, checkMin := s.Limit.PerHour > 0, s.Limit.PerMin > 0
	if !checkHour && !checkMin {
		return
	}
	s.mux.Lock()
	defer s.mux.Unlock()
	releaseHour := checkHour && cleanId&0xff0000 == s.cleanCount&0xff0000
	releaseMin := checkMin && cleanId&0xffff == s.cleanCount&0xffff
	if releaseHour {
		if hour, ok := s.accessHour[id]; ok {
			hour.Add(-1)
		}
	}
	if releaseMin {
		if min, ok := s.accessMin[id]; ok {
			min.Add(-1)
		}
	}
}

func (s *limitSet) clean(hour bool) {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.cleanCount = s.cleanCount&0xff0000 | (s.cleanCount+1)&0xffff
	clear(s.accessMin)
	if hour {
		s.cleanCount = (s.cleanCount+0x10000)&0xff0000 | s.cleanCount&0xffff
		clear(s.accessHour)
	}
}

type APIRateMiddleWare struct {
	realIPContextKey, loggedContextKey any

	annoySet  limitSet
	loggedSet limitSet

	cleanTicker *time.Ticker
	startAt     time.Time
}

var _ utils.MiddleWare = (*APIRateMiddleWare)(nil)

func NewAPIRateMiddleWare(realIPContextKey, loggedContextKey any) (a *APIRateMiddleWare) {
	a = &APIRateMiddleWare{
		loggedContextKey: loggedContextKey,
		annoySet:         makeLimitSet(),
		loggedSet:        makeLimitSet(),
		cleanTicker:      time.NewTicker(time.Minute),
		startAt:          time.Now(),
	}
	go func(ticker *time.Ticker) {
		count := 0
		for range ticker.C {
			count++
			ishour := count > 60
			if ishour {
				count = 0
			}
			a.clean(ishour)
		}
		log.Debugf("cleaner exited")
	}(a.cleanTicker)
	return
}

const (
	RateLimitOverrideContextKey = "go-openbmclapi.limited.rate.api.override"
	RateLimitSkipContextKey     = "go-openbmclapi.limited.rate.api.skip"
)

func SetSkipRateLimit(req *http.Request) *http.Request {
	ctx := req.Context()
	v := ctx.Value(RateLimitSkipContextKey)
	if v == nil {
		return req.WithContext(context.WithValue(ctx, RateLimitSkipContextKey, true))
	}
	if skip, ok := v.(*bool); ok {
		*skip = true
	}
	return req
}

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
	log.Debugf("API rate limiter destroyed")
	a.cleanTicker.Stop()
}

func (a *APIRateMiddleWare) clean(ishour bool) {
	log.Debugf("Cleaning API rate limiter, ishour=%v", ishour)
	a.loggedSet.clean(ishour)
	a.annoySet.clean(ishour)
	log.Debug("API rate limiter cleaned")
}

func (a *APIRateMiddleWare) ServeMiddle(rw http.ResponseWriter, req *http.Request, next http.Handler) {
	ctx := req.Context()
	if ctx.Value(RateLimitSkipContextKey) == true {
		return
	}
	skipPtr := new(bool)
	ctx = context.WithValue(ctx, RateLimitSkipContextKey, skipPtr)

	var set *limitSet
	id, ok := ctx.Value(a.loggedContextKey).(string)
	if ok {
		set = &a.loggedSet
	} else {
		id, _ = ctx.Value(a.realIPContextKey).(string)
		if id == "" {
			id, _, _ = net.SplitHostPort(req.RemoteAddr)
		}
		set = &a.annoySet
	}
	hourLeft, minLeft, cleanId := set.try(id)
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
	if cleanId == 0 {
		rw.Header().Set("Retry-After", strconv.Itoa(retryAfter+1))
		rw.WriteHeader(http.StatusTooManyRequests)
		return
	}

	srw := utils.WrapAsStatusResponseWriter(rw)
	srw.BeforeWriteHeader(func(status int) {
		s := status / 100
		if *skipPtr || s == 3 || s == 1 {
			rw.Header().Set("X-Ratelimit-Remaining-Minute", strconv.FormatInt(minLeft+1, 10))
			rw.Header().Set("X-Ratelimit-Remaining-Hour", strconv.FormatInt(hourLeft+1, 10))
			set.release(id, cleanId)
			return
		}
	})
	next.ServeHTTP(srw, req.WithContext(ctx))
}

func (a *APIRateMiddleWare) WrapHandler(next http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		a.ServeMiddle(rw, req, next)
	}
}
