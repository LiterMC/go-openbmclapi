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

package lang

import (
	"os"
	"strings"
)

type Language struct {
	Lang string
	Area string
	tr   map[string]string
}

func (l *Language) Code() string {
	if l == nil {
		return "<unknown>"
	}
	return l.Lang + "-" + strings.ToUpper(l.Area)
}

func (l *Language) Tr(name string) string {
	if s, ok := l.tr[name]; ok {
		return s
	}
	return "[[" + name + "]]"
}

var languages = make(map[string][]*Language)
var currentLang *Language = nil

func RegisterLanguage(code string, tr map[string]string) {
	code = strings.ToLower(code)
	if len(code) != 5 || (code[2] != '-' && code[2] != '_') {
		panic(code + " is not a language code")
	}
	lang, area := code[:2], code[3:]
	langs := languages[lang]
	for _, l := range langs {
		if l.Area == area {
			panic(code + " is already exists")
		}
	}
	l := &Language{
		Lang: lang,
		Area: area,
		tr:   tr,
	}
	languages[lang] = append(langs, l)
}

func GetLang() *Language {
	return currentLang
}

func Tr(name string) string {
	return currentLang.Tr(name)
}

func SetLang(code string) {
	code = strings.ToLower(code)
	var lang, area string
	if len(code) == 2 {
		lang = code
	} else if len(code) == 5 && (code[2] == '-' || code[2] == '_') {
		lang, area = code[:2], code[3:]
	}
	if lang == "" {
		return
	}
	langs := languages[lang]
	if len(langs) > 0 {
		currentLang = langs[0]
	}
	if area == "" {
		return
	}
	for _, l := range langs {
		if l.Area == area {
			currentLang = l
		}
	}
}

func ParseSystemLanguage() {
	lang, _, _ := strings.Cut(os.Getenv("LANG"), ".")
	if lang != "" {
		SetLang(lang)
	}
	// TODO: syscall.LoadDLL("kernel32") for windows
}
