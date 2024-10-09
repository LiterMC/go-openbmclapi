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

package utils_test

import (
	"testing"

	"encoding/json"
	"reflect"

	"github.com/LiterMC/go-openbmclapi/utils"
)

func TestEmbedJSON(t *testing.T) {
	type testPayload struct {
		A int
		B utils.EmbedJSON[struct {
			C string
			D float64
			E utils.EmbedJSON[*string]
			F *utils.EmbedJSON[string]
		}]
		G utils.EmbedJSON[*string]
		H *utils.EmbedJSON[string]
		I utils.EmbedJSON[*string] `json:",omitempty"`
		J *utils.EmbedJSON[string] `json:",omitempty"`
	}
	var v testPayload
	v.A = 1
	v.B.V.C = "2\""
	v.B.V.D = 3.4
	v.B.V.E.V = new(string)
	*v.B.V.E.V = `{5"6"7}`
	v.B.V.F = &utils.EmbedJSON[string]{
		V: "f",
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	dataStr := (string)(data)
	if want := `{"A":1,"B":"{\"C\":\"2\\\"\",\"D\":3.4,\"E\":\"\\\"{5\\\\\\\"6\\\\\\\"7}\\\"\",\"F\":\"\\\"f\\\"\"}","G":"null","H":null,"I":"null"}`; dataStr != want {
		t.Fatalf("Marshal error, got %s, want %s", dataStr, want)
	}
	var w testPayload
	if err := json.Unmarshal(data, &w); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(w, v) {
		t.Fatalf("Unmarshal error, got %#v, want %#v", w, v)
	}
}
