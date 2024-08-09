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

package email

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"strconv"
	"strings"
	"time"

	mail "github.com/xhit/go-simple-mail/v2"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/notify"
)

//go:embed templates
var tmplFS embed.FS
var tmpl = func() *template.Template {
	t := template.New("")
	t.Funcs(template.FuncMap{
		"noescape": func(v any) (template.HTML, error) {
			switch v := v.(type) {
			case string:
				return (template.HTML)(v), nil
			case []byte:
				return (template.HTML)(v), nil
			default:
				return "", fmt.Errorf("Except string or bytes, got %T", v)
			}
		},
		"tojson": func(v any) (string, error) {
			buf, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return (string)(buf), nil
		},
	})
	template.Must(t.ParseFS(tmplFS, "**/*.gohtml"))
	return t
}()

type Plugin struct {
	db   database.DB
	smtp *mail.SMTPServer
}

var _ notify.Plugin = (*Plugin)(nil)

func NewSMTP(smtpAddr string, smtpType string, username string, password string) (*Plugin, error) {
	smtp := &mail.SMTPServer{
		Authentication: mail.AuthAuto,
		Helo:           "localhost",
		ConnectTimeout: 15 * time.Second,
		SendTimeout:    time.Minute,
		Username:       username,
		Password:       password,
	}
	switch strings.ToLower(smtpType) {
	case "tls", "starttls":
		smtp.Encryption = mail.EncryptionSTARTTLS
	case "ssl", "ssltls":
		smtp.Encryption = mail.EncryptionSSLTLS
	case "none":
		smtp.Encryption = mail.EncryptionNone
	default:
		return nil, fmt.Errorf("Unknown smtp encryption type %q", smtpType)
	}
	var (
		port string
		err  error
	)
	smtp.Host, port, err = net.SplitHostPort(smtpAddr)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse smtp server address: %w", err)
	}
	if smtp.Port, err = strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("Cannot parse smtp server address: %w", err)
	}
	return &Plugin{
		smtp: smtp,
	}, nil
}

func (p *Plugin) ID() string {
	return "email"
}

func (p *Plugin) Init(ctx context.Context, m *notify.Manager) (err error) {
	p.db = m.DB()
	return
}

func (p *Plugin) sendEmail(ctx context.Context, subject string, body []byte, to []string) (err error) {
	cli, err := p.smtp.Connect()
	if err != nil {
		return
	}
	if err = ctx.Err(); err != nil {
		return
	}
	m := mail.NewMSG().
		SetFrom(fmt.Sprintf("Go-OpenBMCLAPI Event Dispatcher <%s>", p.smtp.Username)).
		AddBcc(to...).
		SetSubject(subject).
		SetBodyData(mail.TextHTML, body)
	return m.Send(cli)
}

func (p *Plugin) sendEmailIf(ctx context.Context, subject string, body []byte, filter func(*api.EmailSubscriptionRecord) bool) (err error) {
	var recipients []string
	p.db.ForEachEnabledEmailSubscription(func(record *api.EmailSubscriptionRecord) error {
		if filter(record) {
			recipients = append(recipients, record.Addr)
		}
		return nil
	})
	if err = ctx.Err(); err != nil {
		return
	}
	return p.sendEmail(ctx, subject, body, recipients)
}

func (p *Plugin) OnEnabled(e *notify.EnabledEvent) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "enabled", e); err != nil {
		return err
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return p.sendEmailIf(tctx, "Go-OpenBMCLAPI Enabled", buf.Bytes(), func(record *api.EmailSubscriptionRecord) bool { return record.Scopes.Enabled })
}

func (p *Plugin) OnDisabled(e *notify.DisabledEvent) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "disabled", e); err != nil {
		return err
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return p.sendEmailIf(tctx, "Go-OpenBMCLAPI Disabled", buf.Bytes(), func(record *api.EmailSubscriptionRecord) bool { return record.Scopes.Disabled })
}

func (p *Plugin) OnSyncBegin(e *notify.SyncBeginEvent) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "syncbegin", e); err != nil {
		return err
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return p.sendEmailIf(tctx, "Go-OpenBMCLAPI Sync Begin", buf.Bytes(), func(record *api.EmailSubscriptionRecord) bool { return record.Scopes.SyncBegin })
}

func (p *Plugin) OnSyncDone(e *notify.SyncDoneEvent) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "syncdone", e); err != nil {
		return err
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return p.sendEmailIf(tctx, "Go-OpenBMCLAPI Sync Done", buf.Bytes(), func(record *api.EmailSubscriptionRecord) bool { return record.Scopes.SyncDone })
}

func (p *Plugin) OnUpdateAvaliable(e *notify.UpdateAvaliableEvent) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "updates", e); err != nil {
		return err
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return p.sendEmailIf(tctx, "Go-OpenBMCLAPI Update Avaliable", buf.Bytes(), func(record *api.EmailSubscriptionRecord) bool { return record.Scopes.Updates })
}

func (p *Plugin) OnReportStatus(e *notify.ReportStatusEvent) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "daily-report", e); err != nil {
		return err
	}

	if e.At.Hour() != 22 || e.At.Minute() >= 30 {
		// todo: save report time
		return nil
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return p.sendEmailIf(tctx, "Go-OpenBMCLAPI Daily Report", buf.Bytes(), func(record *api.EmailSubscriptionRecord) bool { return record.Scopes.DailyReport })
}
