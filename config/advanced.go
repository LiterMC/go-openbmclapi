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

package config

type AdvancedConfig struct {
	DebugLog           bool `yaml:"debug-log"`
	SocketIOLog        bool `yaml:"socket-io-log"`
	NoHeavyCheck       bool `yaml:"no-heavy-check"`
	NoGC               bool `yaml:"no-gc"`
	HeavyCheckInterval int  `yaml:"heavy-check-interval"`
	KeepaliveTimeout   int  `yaml:"keepalive-timeout"`
	NoFastEnable       bool `yaml:"no-fast-enable"`
	WaitBeforeEnable   int  `yaml:"wait-before-enable"`

	// DoNotRedirectHTTPSToSecureHostname bool `yaml:"do-NOT-redirect-https-to-SECURE-hostname"`
	DoNotOpenFAQOnWindows              bool `yaml:"do-not-open-faq-on-windows"`
}
