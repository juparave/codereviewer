# Email System - Complete AI Reference

> **AI Agent Instructions**: This is a comprehensive, self-contained reference for implementing a production-ready email system in Go. All code examples are complete and functional. Use this as your primary reference for email-related tasks.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Core Implementation](#core-implementation)
3. [Template System](#template-system)
4. [Caching Strategy](#caching-strategy)
5. [Email Sending (SMTP & AWS SES)](#email-sending)
6. [Attachments & Multipart](#attachments--multipart)
7. [Security & Rate Limiting](#security--rate-limiting)
8. [Production Deployment](#production-deployment)
9. [Testing](#testing)
10. [Configuration Examples](#configuration-examples)

---

## Architecture Overview

### System Design

```
Application Layer (Handlers, Controllers)
           ↓
    Mail Service (Validation, Template Rendering)
           ↓
    Buffered Channel (Async, Non-blocking)
           ↓
    Background Workers (Goroutines)
           ↓
    Provider Interface (SMTP / AWS SES)
           ↓
    Email Delivery
```

### Key Design Decisions

1. **Channel-Based**: Non-blocking email queue with buffered channel
2. **Dual Caching**: Compile-time template embedding + runtime HTML caching
3. **Interface-Based Providers**: Easy to switch between SMTP/SES/others
4. **Type-Safe Templates**: Strongly-typed context structs prevent errors

---

## Core Implementation

### 1. Data Structures

```go
// pkg/mail/types.go
package mail

import "time"

type Email struct {
    To          []string
    Cc          []string
    Bcc         []string
    Subject     string
    Body        string
    Attachments []Attachment
}

type TemplateEmail struct {
    To           []string
    Cc           []string
    Bcc          []string
    Subject      string
    TemplateName string
    Data         interface{}
    Attachments  []Attachment
}

type Attachment struct {
    Filename    string
    ContentType string
    Data        []byte
    Inline      bool
    ContentID   string
}

type EmailResult struct {
    Success   bool
    MessageID string
    Error     error
    SentAt    time.Time
}

type MailData struct {
    Email        *Email
    TemplateData *TemplateEmail
    ResultChan   chan<- EmailResult
}
```

### 2. Main Service

```go
// pkg/mail/service.go
package mail

import (
    "fmt"
    "log"
    "sync"
    "time"
)

type Service struct {
    config      *EmailConfig
    mailChan    chan MailData
    sender      Sender
    templates   *TemplateManager
    cache       *TemplateCache
    logger      *log.Logger
    wg          sync.WaitGroup
    shutdown    chan struct{}
    metrics     *Metrics
}

type EmailConfig struct {
    SMTPHost     string
    SMTPPort     string
    SMTPUser     string
    SMTPPassword string
    FromAddress  string
    FromName     string
    Provider     string
    AWSRegion    string
    AWSAccessKey string
    AWSSecretKey string
    ChannelBuffer int
    EnableAsync   bool
    DevMode       bool
    TestRecipient string
}

func NewService(cfg *EmailConfig, logger *log.Logger) (*Service, error) {
    var sender Sender
    var err error

    switch cfg.Provider {
    case "smtp":
        sender, err = NewSMTPSender(cfg)
    case "aws_ses":
        sender, err = NewSESSender(cfg)
    default:
        return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
    }

    if err != nil {
        return nil, err
    }

    templates, err := NewTemplateManager()
    if err != nil {
        return nil, err
    }

    service := &Service{
        config:    cfg,
        mailChan:  make(chan MailData, cfg.ChannelBuffer),
        sender:    sender,
        templates: templates,
        cache:     NewTemplateCache(10 * time.Minute),
        logger:    logger,
        shutdown:  make(chan struct{}),
        metrics:   NewMetrics(),
    }

    return service, nil
}

func (s *Service) Start() {
    s.wg.Add(1)
    go s.worker()
    s.logger.Println("Email service started")
}

func (s *Service) Stop() {
    close(s.shutdown)
    s.wg.Wait()
    close(s.mailChan)
    s.logger.Println("Email service stopped")
}

func (s *Service) Send(email Email) error {
    if !s.config.EnableAsync {
        result := s.processSend(email)
        return result.Error
    }

    select {
    case s.mailChan <- MailData{Email: &email}:
        return nil
    default:
        s.logger.Println("Email channel full, sending synchronously")
        result := s.processSend(email)
        return result.Error
    }
}

func (s *Service) SendTemplate(email TemplateEmail) error {
    if !s.config.EnableAsync {
        result := s.processTemplateSend(email)
        return result.Error
    }

    select {
    case s.mailChan <- MailData{TemplateData: &email}:
        return nil
    default:
        s.logger.Println("Email channel full, sending synchronously")
        result := s.processTemplateSend(email)
        return result.Error
    }
}

func (s *Service) worker() {
    defer s.wg.Done()

    for {
        select {
        case <-s.shutdown:
            return
        case mailData := <-s.mailChan:
            startTime := time.Now()
            var result EmailResult

            if mailData.Email != nil {
                result = s.processSend(*mailData.Email)
            } else if mailData.TemplateData != nil {
                result = s.processTemplateSend(*mailData.TemplateData)
            }

            duration := time.Since(startTime)
            if result.Success {
                s.metrics.RecordSuccess(duration)
            } else {
                s.metrics.RecordFailure()
            }

            if mailData.ResultChan != nil {
                mailData.ResultChan <- result
            }
        }
    }
}

func (s *Service) processSend(email Email) EmailResult {
    if s.config.DevMode && s.config.TestRecipient != "" {
        email.To = []string{s.config.TestRecipient}
        email.Cc = nil
        email.Bcc = nil
    }

    messageID, err := s.sender.Send(email)
    return EmailResult{
        Success:   err == nil,
        MessageID: messageID,
        Error:     err,
        SentAt:    time.Now(),
    }
}

func (s *Service) processTemplateSend(email TemplateEmail) EmailResult {
    cacheKey := s.cache.GenerateKey(email.TemplateName, email.Data)
    if cached, found := s.cache.Get(cacheKey); found {
        return s.processSend(Email{
            To:          email.To,
            Cc:          email.Cc,
            Bcc:         email.Bcc,
            Subject:     email.Subject,
            Body:        cached,
            Attachments: email.Attachments,
        })
    }

    rendered, err := s.templates.Render(email.TemplateName, email.Data)
    if err != nil {
        return EmailResult{
            Success: false,
            Error:   err,
            SentAt:  time.Now(),
        }
    }

    s.cache.Set(cacheKey, rendered)

    return s.processSend(Email{
        To:          email.To,
        Cc:          email.Cc,
        Bcc:         email.Bcc,
        Subject:     email.Subject,
        Body:        rendered,
        Attachments: email.Attachments,
    })
}
```

---

## Template System

### Template Manager with Compile-Time Embedding

```go
// pkg/mail/templates.go
package mail

import (
    "bytes"
    "embed"
    "fmt"
    "html/template"
    "strings"
    "time"
)

//go:embed templates/*.html
var templatesFS embed.FS

type TemplateManager struct {
    templates map[string]*template.Template
    funcMap   template.FuncMap
}

func NewTemplateManager() (*TemplateManager, error) {
    tm := &TemplateManager{
        templates: make(map[string]*template.Template),
        funcMap:   makeTemplateFuncs(),
    }

    if err := tm.loadTemplates(); err != nil {
        return nil, err
    }

    return tm, nil
}

func (tm *TemplateManager) loadTemplates() error {
    entries, err := templatesFS.ReadDir("templates")
    if err != nil {
        return err
    }

    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
            continue
        }

        name := strings.TrimSuffix(entry.Name(), ".html")
        tmpl := template.New(entry.Name()).Funcs(tm.funcMap)

        tmpl, err = tmpl.ParseFS(templatesFS, "templates/"+entry.Name())
        if err != nil {
            return fmt.Errorf("failed to parse %s: %w", name, err)
        }

        tm.templates[name] = tmpl
    }

    return nil
}

func (tm *TemplateManager) Render(templateName string, data interface{}) (string, error) {
    tmpl, exists := tm.templates[templateName]
    if !exists {
        return "", fmt.Errorf("template not found: %s", templateName)
    }

    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, data); err != nil {
        return "", err
    }

    return buf.String(), nil
}

// Custom template functions
func makeTemplateFuncs() template.FuncMap {
    return template.FuncMap{
        "safeHTML":       safeHTML,
        "formatDate":     formatDate,
        "formatLongDate": formatLongDate,
        "truncate":       truncate,
        "upper":          strings.ToUpper,
        "lower":          strings.ToLower,
        "add":            add,
        "sub":            sub,
    }
}

func safeHTML(s string) template.HTML {
    return template.HTML(s)
}

func formatDate(t time.Time, layout string) string {
    return t.Format(layout)
}

func formatLongDate(t time.Time) string {
    return t.Format("Monday, January 2, 2006")
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen] + "..."
}

func add(a, b int) int { return a + b }
func sub(a, b int) int { return a - b }
```

### Example Template

```html
<!-- pkg/mail/templates/welcome.html -->
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Welcome</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #667eea; color: white; padding: 20px; text-align: center; }
        .content { padding: 20px; }
        @media (max-width: 600px) {
            .container { padding: 10px; }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Welcome to {{.AppName}}</h1>
        </div>
        <div class="content">
            <p>Hello {{.Name | title}},</p>
            <p>Thank you for joining us on {{formatLongDate .CreatedAt}}!</p>
            <p><a href="{{.LoginURL}}">Login to your account</a></p>
        </div>
    </div>
</body>
</html>
```

---

## Caching Strategy

### Runtime Template Cache

```go
// pkg/mail/cache.go
package mail

import (
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "sync"
    "time"
)

type CacheEntry struct {
    Content   string
    CreatedAt time.Time
    ExpiresAt time.Time
}

type TemplateCache struct {
    mu      sync.RWMutex
    cache   map[string]*CacheEntry
    ttl     time.Duration
    maxSize int
}

func NewTemplateCache(ttl time.Duration) *TemplateCache {
    cache := &TemplateCache{
        cache:   make(map[string]*CacheEntry),
        ttl:     ttl,
        maxSize: 1000,
    }

    go cache.cleanup()
    return cache
}

func (tc *TemplateCache) GenerateKey(templateName string, data interface{}) string {
    dataJSON, err := json.Marshal(data)
    if err != nil {
        return fmt.Sprintf("%s:%d", templateName, time.Now().UnixNano())
    }

    hash := sha256.Sum256(append([]byte(templateName), dataJSON...))
    return fmt.Sprintf("%x", hash[:16])
}

func (tc *TemplateCache) Get(key string) (string, bool) {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    entry, exists := tc.cache[key]
    if !exists || time.Now().After(entry.ExpiresAt) {
        return "", false
    }

    return entry.Content, true
}

func (tc *TemplateCache) Set(key, content string) {
    tc.mu.Lock()
    defer tc.mu.Unlock()

    if len(tc.cache) >= tc.maxSize {
        tc.evictOldest()
    }

    tc.cache[key] = &CacheEntry{
        Content:   content,
        CreatedAt: time.Now(),
        ExpiresAt: time.Now().Add(tc.ttl),
    }
}

func (tc *TemplateCache) evictOldest() {
    var oldestKey string
    var oldestTime time.Time

    for key, entry := range tc.cache {
        if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
            oldestKey = key
            oldestTime = entry.CreatedAt
        }
    }

    if oldestKey != "" {
        delete(tc.cache, oldestKey)
    }
}

func (tc *TemplateCache) cleanup() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        tc.mu.Lock()
        now := time.Now()
        for key, entry := range tc.cache {
            if now.After(entry.ExpiresAt) {
                delete(tc.cache, key)
            }
        }
        tc.mu.Unlock()
    }
}
```

---

## Email Sending

### Sender Interface

```go
// pkg/mail/sender.go
package mail

type Sender interface {
    Send(email Email) (messageID string, err error)
    SendBatch(emails []Email) (results []EmailResult, err error)
    Validate() error
}
```

### SMTP Implementation

```go
// pkg/mail/smtp_sender.go
package mail

import (
    "bytes"
    "crypto/tls"
    "fmt"
    "net"
    "net/smtp"
    "strings"
    "time"
)

type SMTPSender struct {
    config *EmailConfig
    auth   smtp.Auth
    addr   string
    logger *log.Logger
}

func NewSMTPSender(cfg *EmailConfig) (*SMTPSender, error) {
    sender := &SMTPSender{
        config: cfg,
        addr:   net.JoinHostPort(cfg.SMTPHost, cfg.SMTPPort),
        logger: log.New(os.Stdout, "[SMTP] ", log.LstdFlags),
    }

    sender.auth = smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPHost)

    if err := sender.Validate(); err != nil {
        return nil, err
    }

    return sender, nil
}

func (s *SMTPSender) Send(email Email) (string, error) {
    message, err := s.buildMessage(email)
    if err != nil {
        return "", err
    }

    recipients := append(email.To, email.Cc...)
    recipients = append(recipients, email.Bcc...)

    // Retry logic
    var lastErr error
    for attempt := 1; attempt <= 3; attempt++ {
        err = s.sendWithTimeout(recipients, message, 30*time.Second)
        if err == nil {
            messageID := s.extractMessageID(message)
            return messageID, nil
        }

        lastErr = err
        if attempt < 3 {
            time.Sleep(time.Duration(attempt*attempt) * time.Second)
        }
    }

    return "", lastErr
}

func (s *SMTPSender) sendWithTimeout(recipients []string, message []byte, timeout time.Duration) error {
    conn, err := net.DialTimeout("tcp", s.addr, timeout)
    if err != nil {
        return err
    }
    defer conn.Close()

    conn.SetDeadline(time.Now().Add(timeout))

    client, err := smtp.NewClient(conn, s.config.SMTPHost)
    if err != nil {
        return err
    }
    defer client.Quit()

    if s.config.SMTPPort == "587" {
        tlsConfig := &tls.Config{ServerName: s.config.SMTPHost}
        if err = client.StartTLS(tlsConfig); err != nil {
            return err
        }
    }

    if err = client.Auth(s.auth); err != nil {
        return err
    }

    if err = client.Mail(s.config.FromAddress); err != nil {
        return err
    }

    for _, recipient := range recipients {
        if err = client.Rcpt(recipient); err != nil {
            return err
        }
    }

    writer, err := client.Data()
    if err != nil {
        return err
    }

    _, err = writer.Write(message)
    if err != nil {
        return err
    }

    return writer.Close()
}

func (s *SMTPSender) buildMessage(email Email) ([]byte, error) {
    var buf bytes.Buffer

    headers := map[string]string{
        "From":       fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromAddress),
        "To":         strings.Join(email.To, ", "),
        "Subject":    email.Subject,
        "MIME-Version": "1.0",
        "Date":       time.Now().Format(time.RFC1123Z),
        "Message-ID": fmt.Sprintf("<%d@%s>", time.Now().UnixNano(), s.config.SMTPHost),
    }

    for key, value := range headers {
        buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
    }

    if len(email.Attachments) > 0 {
        return s.buildMultipartMessage(email)
    }

    buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
    buf.WriteString(email.Body)

    return buf.Bytes(), nil
}

func (s *SMTPSender) Validate() error {
    conn, err := net.DialTimeout("tcp", s.addr, 10*time.Second)
    if err != nil {
        return err
    }
    conn.Close()
    return nil
}

func (s *SMTPSender) SendBatch(emails []Email) ([]EmailResult, error) {
    results := make([]EmailResult, len(emails))
    for i, email := range emails {
        messageID, err := s.Send(email)
        results[i] = EmailResult{
            Success:   err == nil,
            MessageID: messageID,
            Error:     err,
            SentAt:    time.Now(),
        }
    }
    return results, nil
}

func (s *SMTPSender) extractMessageID(message []byte) string {
    lines := strings.Split(string(message), "\r\n")
    for _, line := range lines {
        if strings.HasPrefix(line, "Message-ID: ") {
            return strings.TrimPrefix(line, "Message-ID: ")
        }
    }
    return "unknown"
}
```

### AWS SES Implementation

```go
// pkg/mail/ses_sender.go
package mail

import (
    "fmt"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/credentials"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/ses"
)

type SESSender struct {
    config *EmailConfig
    client *ses.SES
    logger *log.Logger
}

func NewSESSender(cfg *EmailConfig) (*SESSender, error) {
    sess, err := session.NewSession(&aws.Config{
        Region: aws.String(cfg.AWSRegion),
        Credentials: credentials.NewStaticCredentials(
            cfg.AWSAccessKey,
            cfg.AWSSecretKey,
            "",
        ),
    })

    if err != nil {
        return nil, err
    }

    sender := &SESSender{
        config: cfg,
        client: ses.New(sess),
        logger: log.New(os.Stdout, "[SES] ", log.LstdFlags),
    }

    return sender, nil
}

func (s *SESSender) Send(email Email) (string, error) {
    input := &ses.SendEmailInput{
        Source: aws.String(fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromAddress)),
        Destination: &ses.Destination{
            ToAddresses:  aws.StringSlice(email.To),
            CcAddresses:  aws.StringSlice(email.Cc),
            BccAddresses: aws.StringSlice(email.Bcc),
        },
        Message: &ses.Message{
            Subject: &ses.Content{Data: aws.String(email.Subject)},
            Body:    &ses.Body{Html: &ses.Content{Data: aws.String(email.Body)}},
        },
    }

    result, err := s.client.SendEmail(input)
    if err != nil {
        return "", err
    }

    return aws.StringValue(result.MessageId), nil
}

func (s *SESSender) Validate() error {
    input := &ses.GetIdentityVerificationAttributesInput{
        Identities: []*string{aws.String(s.config.FromAddress)},
    }

    result, err := s.client.GetIdentityVerificationAttributes(input)
    if err != nil {
        return err
    }

    attr, exists := result.VerificationAttributes[s.config.FromAddress]
    if !exists || aws.StringValue(attr.VerificationStatus) != "Success" {
        return fmt.Errorf("sender email not verified: %s", s.config.FromAddress)
    }

    return nil
}

func (s *SESSender) SendBatch(emails []Email) ([]EmailResult, error) {
    results := make([]EmailResult, len(emails))
    for i, email := range emails {
        messageID, err := s.Send(email)
        results[i] = EmailResult{
            Success:   err == nil,
            MessageID: messageID,
            Error:     err,
            SentAt:    time.Now(),
        }
    }
    return results, nil
}
```

---

## Attachments & Multipart

### Multipart Message Builder

```go
// pkg/mail/multipart.go
package mail

import (
    "bytes"
    "encoding/base64"
    "fmt"
    "mime/multipart"
    "net/textproto"
)

func (s *SMTPSender) buildMultipartMessage(email Email) ([]byte, error) {
    var buf bytes.Buffer
    writer := multipart.NewWriter(&buf)

    // Write headers
    headers := map[string]string{
        "From":         fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromAddress),
        "To":           strings.Join(email.To, ", "),
        "Subject":      email.Subject,
        "MIME-Version": "1.0",
        "Content-Type": fmt.Sprintf("multipart/mixed; boundary=%s", writer.Boundary()),
    }

    for key, value := range headers {
        buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
    }
    buf.WriteString("\r\n")

    // HTML body
    htmlPart, _ := writer.CreatePart(textproto.MIMEHeader{
        "Content-Type": {"text/html; charset=UTF-8"},
    })
    htmlPart.Write([]byte(email.Body))

    // Attachments
    for _, att := range email.Attachments {
        headers := textproto.MIMEHeader{}
        headers.Set("Content-Type", att.ContentType)
        headers.Set("Content-Transfer-Encoding", "base64")

        if att.Inline && att.ContentID != "" {
            headers.Set("Content-ID", fmt.Sprintf("<%s>", att.ContentID))
            headers.Set("Content-Disposition", "inline")
        } else {
            headers.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.Filename))
        }

        part, _ := writer.CreatePart(headers)
        encoder := base64.NewEncoder(base64.StdEncoding, part)
        encoder.Write(att.Data)
        encoder.Close()
    }

    writer.Close()
    return buf.Bytes(), nil
}
```

---

## Security & Rate Limiting

### Rate Limiter

```go
// pkg/mail/ratelimit.go
package mail

import (
    "fmt"
    "sync"
    "time"
)

type RateLimiter struct {
    mu         sync.RWMutex
    userLimits map[string]*userLimit
    maxPerUser int
    window     time.Duration
}

type userLimit struct {
    count     int
    firstSent time.Time
}

func NewRateLimiter(maxPerUser int, window time.Duration) *RateLimiter {
    return &RateLimiter{
        userLimits: make(map[string]*userLimit),
        maxPerUser: maxPerUser,
        window:     window,
    }
}

func (rl *RateLimiter) Allow(userID string) error {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    now := time.Now()
    limit, exists := rl.userLimits[userID]

    if !exists {
        rl.userLimits[userID] = &userLimit{count: 1, firstSent: now}
        return nil
    }

    if now.Sub(limit.firstSent) > rl.window {
        limit.count = 1
        limit.firstSent = now
        return nil
    }

    if limit.count >= rl.maxPerUser {
        return fmt.Errorf("rate limit exceeded: %d emails in %v", limit.count, rl.window)
    }

    limit.count++
    return nil
}
```

### Input Validation

```go
// pkg/mail/validation.go
package mail

import (
    "fmt"
    "net/mail"
    "strings"
)

func ValidateEmail(email string) error {
    if email == "" {
        return fmt.Errorf("email cannot be empty")
    }

    addr, err := mail.ParseAddress(email)
    if err != nil {
        return fmt.Errorf("invalid email: %w", err)
    }

    if !strings.Contains(addr.Address, "@") {
        return fmt.Errorf("email must contain @")
    }

    return nil
}

func ValidateEmailList(emails []string) error {
    if len(emails) == 0 {
        return fmt.Errorf("recipient list cannot be empty")
    }

    for _, email := range emails {
        if err := ValidateEmail(email); err != nil {
            return err
        }
    }

    return nil
}
```

---

## Production Deployment

### Metrics Collection

```go
// pkg/mail/metrics.go
package mail

import (
    "sync"
    "time"
)

type Metrics struct {
    mu              sync.RWMutex
    TotalSent       int64
    TotalFailed     int64
    AverageSendTime time.Duration
    sendTimes       []time.Duration
}

func NewMetrics() *Metrics {
    return &Metrics{
        sendTimes: make([]time.Duration, 0, 1000),
    }
}

func (m *Metrics) RecordSuccess(duration time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.TotalSent++
    m.sendTimes = append(m.sendTimes, duration)

    if len(m.sendTimes) > 1000 {
        m.sendTimes = m.sendTimes[len(m.sendTimes)-1000:]
    }

    var total time.Duration
    for _, t := range m.sendTimes {
        total += t
    }
    m.AverageSendTime = total / time.Duration(len(m.sendTimes))
}

func (m *Metrics) RecordFailure() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.TotalFailed++
}

func (m *Metrics) Stats() map[string]interface{} {
    m.mu.RLock()
    defer m.mu.RUnlock()

    total := m.TotalSent + m.TotalFailed
    successRate := float64(0)
    if total > 0 {
        successRate = float64(m.TotalSent) / float64(total) * 100
    }

    return map[string]interface{}{
        "total_sent":       m.TotalSent,
        "total_failed":     m.TotalFailed,
        "success_rate":     successRate,
        "avg_send_time_ms": m.AverageSendTime.Milliseconds(),
    }
}
```

### Environment Configuration

```bash
# Production .env
EMAIL_PROVIDER=aws_ses
EMAIL_FROM_ADDRESS=noreply@yourdomain.com
EMAIL_FROM_NAME=Your Application

# AWS SES
AWS_REGION=us-east-1
AWS_ACCESS_KEY=your-access-key
AWS_SECRET_KEY=your-secret-key

# Operational
EMAIL_CHANNEL_BUFFER=500
EMAIL_ASYNC=true
EMAIL_DEV_MODE=false

# Rate Limiting
EMAIL_RATE_LIMIT_PER_USER=10
EMAIL_RATE_LIMIT_WINDOW=1h
```

---

## Testing

### Mock Sender for Tests

```go
// pkg/mail/mock_sender.go
package mail

import (
    "fmt"
    "sync"
    "time"
)

type MockSender struct {
    mu           sync.Mutex
    SentEmails   []Email
    ShouldFail   bool
    FailureError error
}

func NewMockSender() *MockSender {
    return &MockSender{
        SentEmails: make([]Email, 0),
    }
}

func (m *MockSender) Send(email Email) (string, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.ShouldFail {
        if m.FailureError != nil {
            return "", m.FailureError
        }
        return "", fmt.Errorf("mock failure")
    }

    m.SentEmails = append(m.SentEmails, email)
    return fmt.Sprintf("mock-%d", len(m.SentEmails)), nil
}

func (m *MockSender) SendBatch(emails []Email) ([]EmailResult, error) {
    results := make([]EmailResult, len(emails))
    for i, email := range emails {
        msgID, err := m.Send(email)
        results[i] = EmailResult{
            Success:   err == nil,
            MessageID: msgID,
            Error:     err,
            SentAt:    time.Now(),
        }
    }
    return results, nil
}

func (m *MockSender) Validate() error {
    return nil
}

func (m *MockSender) Reset() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.SentEmails = make([]Email, 0)
}
```

### Test Example

```go
// pkg/mail/service_test.go
package mail_test

import (
    "testing"
    "your-project/pkg/mail"
)

func TestSendEmail(t *testing.T) {
    mockSender := mail.NewMockSender()

    config := &mail.EmailConfig{
        FromAddress:   "test@example.com",
        ChannelBuffer: 10,
        EnableAsync:   false,
    }

    service := &mail.Service{
        config:   config,
        sender:   mockSender,
        mailChan: make(chan mail.MailData, 10),
    }

    email := mail.Email{
        To:      []string{"user@example.com"},
        Subject: "Test",
        Body:    "<h1>Test</h1>",
    }

    err := service.Send(email)
    if err != nil {
        t.Fatalf("Send failed: %v", err)
    }

    if len(mockSender.SentEmails) != 1 {
        t.Errorf("Expected 1 sent email, got %d", len(mockSender.SentEmails))
    }
}
```

---

## Configuration Examples

### Complete Initialization

```go
// main.go - Complete setup example
package main

import (
    "log"
    "os"
    "time"
    "your-project/pkg/mail"
)

func main() {
    // Load configuration
    config := &mail.EmailConfig{
        Provider:      os.Getenv("EMAIL_PROVIDER"),
        SMTPHost:      os.Getenv("EMAIL_HOST"),
        SMTPPort:      os.Getenv("EMAIL_PORT"),
        SMTPUser:      os.Getenv("EMAIL_USER"),
        SMTPPassword:  os.Getenv("EMAIL_PASSWORD"),
        FromAddress:   os.Getenv("EMAIL_FROM_ADDRESS"),
        FromName:      os.Getenv("EMAIL_FROM_NAME"),
        AWSRegion:     os.Getenv("AWS_REGION"),
        AWSAccessKey:  os.Getenv("AWS_ACCESS_KEY"),
        AWSSecretKey:  os.Getenv("AWS_SECRET_KEY"),
        ChannelBuffer: 100,
        EnableAsync:   true,
        DevMode:       os.Getenv("ENV") == "development",
    }

    // Create logger
    logger := log.New(os.Stdout, "[EMAIL] ", log.LstdFlags)

    // Initialize service
    mailService, err := mail.NewService(config, logger)
    if err != nil {
        log.Fatalf("Failed to initialize email service: %v", err)
    }

    // Start background worker
    mailService.Start()
    defer mailService.Stop()

    // Send welcome email
    mailService.SendTemplate(mail.TemplateEmail{
        To:           []string{"user@example.com"},
        Subject:      "Welcome!",
        TemplateName: "welcome",
        Data: map[string]interface{}{
            "Name":      "John Doe",
            "AppName":   "My App",
            "CreatedAt": time.Now(),
            "LoginURL":  "https://example.com/login",
        },
    })

    // Send email with attachment
    pdfData := []byte("PDF content here")
    mailService.Send(mail.Email{
        To:      []string{"user@example.com"},
        Subject: "Invoice",
        Body:    "<h1>Your invoice is attached</h1>",
        Attachments: []mail.Attachment{
            {
                Filename:    "invoice.pdf",
                ContentType: "application/pdf",
                Data:        pdfData,
            },
        },
    })

    // Keep running
    select {}
}
```

---

## Quick Reference Commands

### Provider Setup

**Gmail SMTP:**
```bash
EMAIL_PROVIDER=smtp
EMAIL_HOST=smtp.gmail.com
EMAIL_PORT=587
EMAIL_USER=your-email@gmail.com
EMAIL_PASSWORD=your-16-char-app-password
```

**AWS SES:**
```bash
EMAIL_PROVIDER=aws_ses
AWS_REGION=us-east-1
AWS_ACCESS_KEY=your-access-key
AWS_SECRET_KEY=your-secret-key

# Verify domain
aws ses verify-domain-identity --domain yourdomain.com
```

### Common Patterns

**Send basic email:**
```go
mailService.Send(mail.Email{
    To:      []string{"user@example.com"},
    Subject: "Hello",
    Body:    "<h1>Hello World</h1>",
})
```

**Send template email:**
```go
mailService.SendTemplate(mail.TemplateEmail{
    To:           []string{"user@example.com"},
    Subject:      "Welcome",
    TemplateName: "welcome",
    Data:         userData,
})
```

**Send with attachment:**
```go
mailService.Send(mail.Email{
    To:      []string{"user@example.com"},
    Subject: "Document",
    Body:    "<p>See attachment</p>",
    Attachments: []mail.Attachment{{
        Filename:    "doc.pdf",
        ContentType: "application/pdf",
        Data:        pdfBytes,
    }},
})
```

---

## AI Agent Usage Notes

1. **Always validate emails** before sending
2. **Use templates** for consistent formatting
3. **Enable async sending** in production (set EnableAsync: true)
4. **Implement rate limiting** to prevent abuse
5. **Start workers** before sending any emails
6. **Gracefully shutdown** with defer mailService.Stop()
7. **Use mock sender** in tests
8. **Cache rendered templates** for performance
9. **Embed templates** at compile time with go:embed
10. **Monitor metrics** in production (send count, failure rate, latency)

This reference contains all essential patterns for implementing production-ready email systems in Go.
