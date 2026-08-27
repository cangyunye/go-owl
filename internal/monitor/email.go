package monitor

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// EmailSender 邮件发送抽象（默认 smtp.SendMail，测试可注入 fake）。
type EmailSender interface {
	SendMail(addr string, auth smtp.Auth, from string, to []string, msg []byte) error
}

type smtpSendAdapter struct{}

func (smtpSendAdapter) SendMail(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	return smtp.SendMail(addr, auth, from, to, msg)
}

// EmailNotifier SMTP 邮件通知（重点告警推送）。
type EmailNotifier struct {
	sender EmailSender
}

// NewEmailNotifier 创建邮件通知器。
func NewEmailNotifier() *EmailNotifier {
	return &EmailNotifier{sender: smtpSendAdapter{}}
}

// Send 组装并发送告警邮件。
func (n *EmailNotifier) Send(ctx context.Context, ch NotifyChannel, event AlertEvent, at AlertType, nodeName, webURL string) error {
	cfg := ch.Config.Email
	if cfg == nil {
		return fmt.Errorf("邮件配置缺失")
	}
	port := cfg.SMTPPort
	if port == 0 {
		port = 465
	}
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, port)
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)
	}
	msg := BuildEmailMessage(cfg, event, at, nodeName, webURL)
	if err := n.sender.SendMail(addr, auth, cfg.From, cfg.To, msg); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}
	return nil
}

// BuildEmailMessage 组装 MIME 邮件（标题含级别与告警名，正文含关键信息）。
func BuildEmailMessage(cfg *EmailConfig, event AlertEvent, at AlertType, nodeName, webURL string) []byte {
	a := event.Alert
	subject := fmt.Sprintf("[owl] 告警 %s: %s", a.Severity, at.Name)

	var b strings.Builder
	b.WriteString("From: " + cfg.From + "\r\n")
	b.WriteString("To: " + strings.Join(cfg.To, ", ") + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(fmt.Sprintf("告警 ID: %s\r\n", a.ID))
	b.WriteString(fmt.Sprintf("告警类型: %s (%s)\r\n", a.AlertTypeID, at.Name))
	b.WriteString(fmt.Sprintf("节点: %s (%s)\r\n", nodeName, a.NodeID))
	b.WriteString(fmt.Sprintf("级别: %s\r\n", a.Severity))
	b.WriteString(fmt.Sprintf("状态: %s\r\n", a.Status))
	b.WriteString(fmt.Sprintf("首次触发: %d\r\n", a.FirstSeen))
	b.WriteString(fmt.Sprintf("指标快照: %s\r\n", a.MetricSnapshot))
	b.WriteString(fmt.Sprintf("描述: %s\r\n", a.Message))
	if webURL != "" {
		b.WriteString(fmt.Sprintf("处理入口: %s\r\n", webURL))
	}
	return []byte(b.String())
}
