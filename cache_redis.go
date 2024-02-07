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

package main

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	Client  *redis.Client
	Context context.Context
}

var _ Cache = (*RedisCache)(nil)

func NewRedisCache(opt *redis.Options) *RedisCache {
	return NewRedisCacheByClient(redis.NewClient(opt))
}

func NewRedisCacheByClient(cli *redis.Client) *RedisCache {
	return &RedisCache{
		Client:  cli,
		Context: context.Background(),
	}
}

func (c *RedisCache) Set(key string, value string, opt CacheOpt) {
	ctx, cancel := context.WithTimeout(c.Context, time.Second*3)
	defer cancel()
	c.Client.Set(ctx, key, value, opt.Expiration)
}

func (c *RedisCache) Get(key string) (value string, ok bool) {
	ctx, cancel := context.WithTimeout(c.Context, time.Second)
	defer cancel()
	cmd := c.Client.Get(ctx, key)
	if cmd.Err() != nil {
		return "", false
	}
	return cmd.Val(), true
}
