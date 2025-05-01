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
	Anonymous limited.RateLimit `json:"anonymous" yaml:"anonymous"`
	Logged    limited.RateLimit `json:"logged" yaml:"logged"`
}

type NotificationConfig struct {
	EnableEmail         bool   `json:"enable_email" yaml:"enable-email"`
	EmailSMTP           string `json:"email_smtp" yaml:"email-smtp"`
	EmailSMTPEncryption string `json:"email_smtp_encryption" yaml:"email-smtp-encryption"`
	EmailSender         string `json:"email_sender" yaml:"email-sender"`
	EmailSenderPassword string `json:"email_sender_password" yaml:"email-sender-password"`
	EnableWebhook       bool   `json:"enable_webhook" yaml:"enable-webhook"`
}

type DashboardConfig struct {
	Enable       bool   `json:"enable" yaml:"enable"`
	Username     string `json:"username" yaml:"username"`
	Password     string `json:"password" yaml:"password"`
	PwaName      string `json:"pwa_name" yaml:"pwa-name"`
	PwaShortName string `json:"pwa_short_name" yaml:"pwa-short_name"`
	PwaDesc      string `json:"pwa_description" yaml:"pwa-description"`

	NotifySubject string `json:"notification_subject" yaml:"notification-subject"`
}
