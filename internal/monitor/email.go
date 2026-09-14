package monitor

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailSender 邮件发送抽象（可注入 fake 测试）。
// mode 为加密模式：ssl（隐式 TLS，465）/ starttls / none。
type EmailSender interface {
	Send(mode, addr string, auth smtp.Auth, from string, to []string, msg []byte) error
}

// smtpSendAdapter 默认实现：按加密模式拨号并完成 SMTP 会话。
type smtpSendAdapter struct {
	dialTimeout time.Duration
}

func (a smtpSendAdapter) Send(mode, addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	timeout := a.dialTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	host := hostOf(addr)
	switch mode {
	case "ssl":
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr,
			&tls.Config{ServerName: host})
		if err != nil {
			return fmt.Errorf("SSL 连接失败: %w", err)
		}
		defer func() { _ = conn.Close() }()
		cl, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("SMTP 会话建立失败: %w", err)
		}
		return sendWithClient(cl, auth, from, to, msg)
	case "none":
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return fmt.Errorf("连接失败: %w", err)
		}
		defer func() { _ = conn.Close() }()
		cl, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("SMTP 会话建立失败: %w", err)
		}
		return sendWithClient(cl, auth, from, to, msg)
	default: // starttls：net/smtp.SendMail 自动协商 STARTTLS
		return smtp.SendMail(addr, auth, from, to, msg)
	}
}

// sendWithClient 在已建立的 SMTP 客户端上完成认证与投递。
func sendWithClient(cl *smtp.Client, auth smtp.Auth, from string, to []string, msg []byte) error {
	defer func() { _ = cl.Quit() }()
	if auth != nil {
		if ok, _ := cl.Extension("AUTH"); ok {
			if err := cl.Auth(auth); err != nil {
				return fmt.Errorf("SMTP 认证失败: %w", err)
			}
		}
	}
	if err := cl.Mail(from); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	for _, rcpt := range to {
		if err := cl.Rcpt(rcpt); err != nil {
			return fmt.Errorf("设置收件人 %s 失败: %w", rcpt, err)
		}
	}
	w, err := cl.Data()
	if err != nil {
		return fmt.Errorf("开始数据传输失败: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("结束数据传输失败: %w", err)
	}
	return nil
}

func hostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil && h != "" {
		return h
	}
	return addr
}

// resolveEncryption 解析加密模式：显式配置优先；空/非法按端口推断
//（465→ssl，其余→starttls）。
func resolveEncryption(cfg *EmailConfig) string {
	switch strings.ToLower(strings.TrimSpace(cfg.Encryption)) {
	case "ssl":
		return "ssl"
	case "starttls":
		return "starttls"
	case "none":
		return "none"
	default:
		if cfg.SMTPPort == 465 {
			return "ssl"
		}
		return "starttls"
	}
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
	mode := resolveEncryption(cfg)
	if err := n.sender.Send(mode, addr, auth, cfg.From, cfg.To, msg); err != nil {
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
