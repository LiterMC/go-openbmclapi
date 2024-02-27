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

	"github.com/patrickmn/go-cache"
)

type InMemCache struct {
	cache *cache.Cache
}

var _ Cache = (*InMemCache)(nil)

func NewInMemCache() *InMemCache {
	return &InMemCache{
		cache: cache.New(cache.NoExpiration, time.Minute),
	}
}

func (c *InMemCache) Set(key string, value string, opt CacheOpt) {
	c.cache.Set(key, value, opt.Expiration)
}

func (c *InMemCache) Get(key string) (value string, ok bool) {
	v, ok := c.cache.Get(key)
	if !ok {
		return "", false
	}
	value, ok = v.(string)
	return
}

func (c *InMemCache) SetBytes(key string, value []byte, opt CacheOpt) {
	c.cache.Set(key, value, opt.Expiration)
}

func (c *InMemCache) GetBytes(key string) (value []byte, ok bool) {
	v, ok := c.cache.Get(key)
	if !ok {
		return nil, false
	}
	value, ok = v.([]byte)
	return
}

func (c *InMemCache) Delete(key string) {
	c.cache.Delete(key)
}
