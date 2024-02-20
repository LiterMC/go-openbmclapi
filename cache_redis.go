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
	"encoding/base64"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	Client  *redis.Client
	Context context.Context
}

var _ Cache = (*RedisCache)(nil)

type RedisOptions struct {
	Network    string `yaml:"network"`
	Addr       string `yaml:"addr"`
	ClientName string `yaml:"client-name"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
}

func (o RedisOptions) ToRedis() *redis.Options {
	return &redis.Options{
		Network:    o.Network,
		Addr:       o.Addr,
		ClientName: o.ClientName,
		Username:   o.Username,
		Password:   o.Password,
	}
}

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

func (c *RedisCache) SetBytes(key string, value []byte, opt CacheOpt) {
	v := base64.RawStdEncoding.EncodeToString(value)
	c.Set(key, v, opt)
}

func (c *RedisCache) GetBytes(key string) (value []byte, ok bool) {
	v, ok := c.Get(key)
	if !ok {
		return nil, false
	}
	value, err := base64.RawStdEncoding.DecodeString(v)
	if err != nil {
		return nil, false
	}
	return value, true
}

func (c *RedisCache) Delete(key string) {
	ctx, cancel := context.WithTimeout(c.Context, time.Second)
	defer cancel()
	c.Client.Del(ctx)
}
