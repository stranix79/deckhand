package hub

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// sendMail sends a plain-text e-mail through the configured SMTP relay.
// Port 465 = implicit TLS, anything else = STARTTLS when offered.
// In development (DevLogMagicLinks) the message is logged instead.
func (h *Hub) sendMail(to, subject, body string) error {
	if h.cfg.MailHost == "" {
		slog.Info("EMAIL (not sent, no MAIL_HOST)", "to", to, "subject", subject, "body", body)
		return nil
	}
	from, err := mail.ParseAddress(h.cfg.MailFrom)
	if err != nil {
		return fmt.Errorf("MAIL_FROM: %w", err)
	}
	from.Name = "Deckhand"
	msg := strings.Join([]string{
		"From: " + from.String(),
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Date: " + time.Now().Format(time.RFC1123Z),
		"",
		body,
	}, "\r\n")

	addr := net.JoinHostPort(h.cfg.MailHost, fmt.Sprint(h.cfg.MailPort))
	var c *smtp.Client
	if h.cfg.MailPort == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: h.cfg.MailHost, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		c, err = smtp.NewClient(conn, h.cfg.MailHost)
		if err != nil {
			return err
		}
	} else {
		c, err = smtp.Dial(addr)
		if err != nil {
			return err
		}
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: h.cfg.MailHost, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
	}
	defer func() { _ = c.Close() }()
	if h.cfg.MailUser != "" {
		if err := c.Auth(smtp.PlainAuth("", h.cfg.MailUser, h.cfg.MailPassword, h.cfg.MailHost)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
