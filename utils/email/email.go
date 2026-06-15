// Package email provides a simple SMTP email sender with an optional
// stub mode for development.
package email

import (
	"fmt"
	"net/smtp"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Sender sends emails. When cfg.Stub is true it only logs; real SMTP is used
// otherwise.
type Sender struct {
	cfg    config.EmailConfig
	logger *log.LoggerWrapper
}

// New returns a Sender configured from cfg.
func New(cfg config.EmailConfig, logger *log.LoggerWrapper) *Sender {
	return &Sender{cfg: cfg, logger: logger}
}

// Send sends (or logs) a plain-text email.
func (s *Sender) Send(to, subject, body string) error {
	if s.cfg.Stub {
		s.logger.Info(fmt.Sprintf("EMAIL STUB to=%s subject=%q body=%q", to, subject, body))
		return nil
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTP.Host, s.cfg.SMTP.Port)
	var auth smtp.Auth
	if s.cfg.SMTP.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTP.Username, s.cfg.SMTP.Password, s.cfg.SMTP.Host)
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", s.cfg.From, to, subject, body)
	return smtp.SendMail(addr, auth, s.cfg.From, []string{to}, []byte(msg))
}
