package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"time"

	"github.com/juparave/codereviewer/internal/config"
	"github.com/juparave/codereviewer/internal/domain"
	"github.com/juparave/codereviewer/internal/report"
)

// Service handles email notifications
type Service struct {
	config    config.EmailConfig
	logger    *log.Logger
	formatter *report.Formatter
}

// NewService creates a new notification Service
func NewService(cfg config.EmailConfig, logger *log.Logger) (*Service, error) {
	return &Service{
		config:    cfg,
		logger:    logger,
		formatter: report.NewFormatter(""),
	}, nil
}

// SendReport sends the code review report via email
func (s *Service) SendReport(ctx context.Context, rpt *domain.Report) error {
	// Build email content
	htmlBody := s.formatter.ToHTML(rpt)
	subject := s.buildSubject(rpt)

	// Send email
	return s.send(ctx, subject, htmlBody)
}

func (s *Service) buildSubject(rpt *domain.Report) string {
	date := rpt.Date.Format("Jan 2")

	if !rpt.HasFindings() {
		return fmt.Sprintf("[CRA] Daily Review - %s - ✅ All Clear", date)
	}

	findings := rpt.TotalFindings()
	high := rpt.HighCount()

	if high > 0 {
		return fmt.Sprintf("[CRA] Daily Review - %s - ⚠️ %d findings (%d high)", date, findings, high)
	}

	return fmt.Sprintf("[CRA] Daily Review - %s - %d findings", date, findings)
}

func (s *Service) send(ctx context.Context, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)

	// Build message
	message := s.buildMessage(subject, htmlBody)

	// Retry logic
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		err := s.sendWithTimeout(addr, message, 30*time.Second)
		if err == nil {
			return nil
		}

		lastErr = err
		s.logger.Printf("Email attempt %d failed: %v", attempt, err)

		if attempt < 3 {
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
		}
	}

	return fmt.Errorf("failed after 3 attempts: %w", lastErr)
}

func (s *Service) buildMessage(subject, htmlBody string) []byte {
	var buf bytes.Buffer

	// Headers
	buf.WriteString(fmt.Sprintf("From: %s <%s>\r\n", s.config.FromName, s.config.FromAddress))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", s.config.ToAddress))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString(fmt.Sprintf("Message-ID: <%d@%s>\r\n", time.Now().UnixNano(), s.config.SMTPHost))
	buf.WriteString("\r\n")

	// Body
	buf.WriteString(htmlBody)

	return buf.Bytes()
}

func (s *Service) sendWithTimeout(addr string, message []byte, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("connecting to SMTP server: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	client, err := smtp.NewClient(conn, s.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer client.Quit()

	// Start TLS if port is 587
	if s.config.SMTPPort == 587 {
		tlsConfig := &tls.Config{ServerName: s.config.SMTPHost}
		if err = client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("starting TLS: %w", err)
		}
	}

	// Authenticate
	if s.config.SMTPUser != "" && s.config.SMTPPassword != "" {
		auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPassword, s.config.SMTPHost)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("authenticating: %w", err)
		}
	}

	// Set sender
	if err = client.Mail(s.config.FromAddress); err != nil {
		return fmt.Errorf("setting sender: %w", err)
	}

	// Set recipient
	if err = client.Rcpt(s.config.ToAddress); err != nil {
		return fmt.Errorf("setting recipient: %w", err)
	}

	// Send message body
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("getting data writer: %w", err)
	}

	_, err = writer.Write(message)
	if err != nil {
		return fmt.Errorf("writing message: %w", err)
	}

	return writer.Close()
}

// Validate checks if the email configuration is valid
func (s *Service) Validate() error {
	if s.config.SMTPHost == "" {
		return fmt.Errorf("smtp_host is required")
	}
	if s.config.ToAddress == "" {
		return fmt.Errorf("to_address is required")
	}
	if s.config.FromAddress == "" {
		return fmt.Errorf("from_address is required")
	}

	// Check if we can reach the SMTP server
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("cannot reach SMTP server: %w", err)
	}
	conn.Close()

	return nil
}
