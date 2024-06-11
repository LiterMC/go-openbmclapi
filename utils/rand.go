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

import (
	"math/rand"
	"time"
)

var randInt32Ch = func() chan int32 {
	ch := make(chan int32, 64)
	r := rand.New(rand.NewSource(time.Now().Unix()))
	go func() {
		for {
			ch <- r.Int31()
		}
	}()
	return ch
}()

func RandIntn(n int) int {
	rn := <-randInt32Ch
	return (int)(rn) % n
}
