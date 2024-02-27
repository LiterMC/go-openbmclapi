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

package cache

import (
	"time"

	"github.com/gregjones/httpcache"
)

type CacheOpt struct {
	Expiration time.Duration
}

type Cache interface {
	Set(key string, value string, opt CacheOpt)
	Get(key string) (value string, ok bool)
	SetBytes(key string, value []byte, opt CacheOpt)
	GetBytes(key string) (value []byte, ok bool)
	Delete(key string)
}

type noCache struct{}

func (noCache) Set(key string, value string, opt CacheOpt)      {}
func (noCache) Get(key string) (value string, ok bool)          { return "", false }
func (noCache) SetBytes(key string, value []byte, opt CacheOpt) {}
func (noCache) GetBytes(key string) (value []byte, ok bool)     { return nil, false }
func (noCache) Delete(key string)                               {}

var NoCache Cache = noCache{}

type nsCache struct {
	ns    string
	cache Cache
}

func NewCacheWithNamespace(c Cache, ns string) Cache {
	if c == NoCache {
		return NoCache
	}
	return &nsCache{
		ns:    ns,
		cache: c,
	}
}

func (c *nsCache) Set(key string, value string, opt CacheOpt) {
	c.cache.Set(c.ns+key, value, opt)
}

func (c *nsCache) Get(key string) (value string, ok bool) {
	return c.cache.Get(c.ns + key)
}

func (c *nsCache) SetBytes(key string, value []byte, opt CacheOpt) {
	c.cache.SetBytes(c.ns+key, value, opt)
}

func (c *nsCache) GetBytes(key string) (value []byte, ok bool) {
	return c.cache.GetBytes(c.ns + key)
}

func (c *nsCache) Delete(key string) {
	c.cache.Delete(c.ns + key)
}

type httpCacheWrapper struct {
	c Cache
}

func (c httpCacheWrapper) Set(key string, value []byte) {
	c.c.SetBytes(key, value, CacheOpt{})
}

func (c httpCacheWrapper) Get(key string) (value []byte, ok bool) {
	return c.c.GetBytes(key)
}

func (c httpCacheWrapper) Delete(key string) {
	c.c.Delete(key)
}

func WrapToHTTPCache(c Cache) httpcache.Cache {
	return httpCacheWrapper{c}
}
