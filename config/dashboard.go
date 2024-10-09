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

import (
	"github.com/LiterMC/go-openbmclapi/limited"
)

type APIRateLimitConfig struct {
	Anonymous limited.RateLimit `yaml:"anonymous"`
	Logged    limited.RateLimit `yaml:"logged"`
}

type NotificationConfig struct {
	EnableEmail         bool   `yaml:"enable-email"`
	EmailSMTP           string `yaml:"email-smtp"`
	EmailSMTPEncryption string `yaml:"email-smtp-encryption"`
	EmailSender         string `yaml:"email-sender"`
	EmailSenderPassword string `yaml:"email-sender-password"`
	EnableWebhook       bool   `yaml:"enable-webhook"`
}

type DashboardConfig struct {
	Enable       bool   `yaml:"enable"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	PwaName      string `yaml:"pwa-name"`
	PwaShortName string `yaml:"pwa-short_name"`
	PwaDesc      string `yaml:"pwa-description"`

	NotifySubject string `yaml:"notification-subject"`
}
