// Package service — Email(SMTP) 通知 Provider。
package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
)

// EmailProvider 通过 SMTP 发送邮件通知。
type EmailProvider struct{}

// Send 通过 SMTP 发送邮件。
func (p *EmailProvider) Send(ctx context.Context, cfg map[string]string, event NotifyEvent) error {
	smtpHost := cfg["smtp_host"]
	smtpPortStr := cfg["smtp_port"]
	username := cfg["username"]
	password := cfg["password"]
	from := cfg["from"]
	to := cfg["to"]
	tlsStr := cfg["tls"]

	if smtpHost == "" || smtpPortStr == "" || username == "" || from == "" || to == "" {
		return fmt.Errorf("email: smtp_host, smtp_port, username, from, and to are required")
	}

	smtpPort, err := strconv.Atoi(smtpPortStr)
	if err != nil {
		return fmt.Errorf("email: invalid smtp_port: %s", smtpPortStr)
	}

	useTLS := true
	if tlsStr == "false" || tlsStr == "0" || tlsStr == "no" {
		useTLS = false
	}

	// 构建邮件内容
	subject := fmt.Sprintf("[MediaStationGo] %s", event.Title)
	body := event.Message
	if len(event.Data) > 0 {
		body += "\n\n---\n详细信息:\n"
		for k, v := range event.Data {
			body += fmt.Sprintf("  %s: %v\n", k, v)
		}
	}

	recipients := strings.Split(to, ",")
	for i, r := range recipients {
		recipients[i] = strings.TrimSpace(r)
	}

	// 构建邮件
	fromAddr := mail.Address{Name: "MediaStationGo", Address: from}
	toAddrs := make([]mail.Address, 0, len(recipients))
	for _, r := range recipients {
		toAddrs = append(toAddrs, mail.Address{Address: r})
	}

	msg := "From: " + fromAddr.String() + "\r\n"
	msg += "To: "
	for i, addr := range toAddrs {
		if i > 0 {
			msg += ", "
		}
		msg += addr.String()
	}
	msg += "\r\n"
	msg += "Subject: " + subject + "\r\n"
	msg += "MIME-Version: 1.0\r\n"
	msg += "Content-Type: text/plain; charset=\"utf-8\"\r\n"
	msg += "Content-Transfer-Encoding: base64\r\n"
	msg += "\r\n"
	msg += body

	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	auth := smtp.PlainAuth("", username, password, smtpHost)

	if useTLS {
		// 使用 TLS 连接
		tlsConfig := &tls.Config{
			ServerName: smtpHost,
			MinVersion: tls.VersionTLS12,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("email tls dial: %w", err)
		}
		client, err := smtp.NewClient(conn, smtpHost)
		if err != nil {
			return fmt.Errorf("email smtp client: %w", err)
		}
		defer client.Close()

		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("email auth: %w", err)
		}
		if err = client.Mail(from); err != nil {
			return fmt.Errorf("email mail from: %w", err)
		}
		for _, r := range recipients {
			if err = client.Rcpt(r); err != nil {
				return fmt.Errorf("email rcpt to: %w", err)
			}
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("email data: %w", err)
		}
		if _, err = w.Write([]byte(msg)); err != nil {
			return fmt.Errorf("email write: %w", err)
		}
		if err = w.Close(); err != nil {
			return fmt.Errorf("email close: %w", err)
		}
		return client.Quit()
	}

	// 不使用 TLS（STARTTLS 或明文）
	return smtp.SendMail(addr, auth, from, recipients, []byte(msg))
}

// ValidateConfig 验证 Email 配置。
func (p *EmailProvider) ValidateConfig(cfg map[string]string) error {
	if cfg["smtp_host"] == "" {
		return fmt.Errorf("email: smtp_host is required")
	}
	if cfg["smtp_port"] == "" {
		return fmt.Errorf("email: smtp_port is required")
	}
	if cfg["username"] == "" {
		return fmt.Errorf("email: username is required")
	}
	if cfg["from"] == "" {
		return fmt.Errorf("email: from is required")
	}
	if cfg["to"] == "" {
		return fmt.Errorf("email: to is required")
	}
	if _, err := strconv.Atoi(cfg["smtp_port"]); err != nil {
		return fmt.Errorf("email: invalid smtp_port")
	}
	return nil
}
