
/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
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
	"testing"
)

func TestIsHex(t *testing.T){
	var data = []struct{
		S string
		B bool
	}{
		{"", false},
		{"0", false},
		{"00", true},
		{"000", false},
		{"0f", true},
		{"f0", true},
		{"af", true},
		{"fa", true},
		{"ff", true},
		{"aa", true},
		{"11", true},
		{"fg", false},
		{"58fe4669c65dbde8e0d58afb83e21620", true},
		{"1cad5d7f9ed285a04784429ab2a4615d5c59ea88", true},
	}
	for _, d := range data {
		ok := IsHex(d.S)
		if ok != d.B {
			t.Errorf("IsHex(%q) returned %v, but expected %v", d.S, ok, d.B)
		}
	}
}
