package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/conductor/conductor/internal/database"
)

// EmailChannel implements the Channel interface for email notifications.
type EmailChannel struct {
	config    EmailConfig
	client    *smtpClient
	logger    *slog.Logger
	mu        sync.Mutex
	lastUsed  time.Time
	maxAge    time.Duration
	htmlTmpl  *template.Template
	plainTmpl *template.Template
}

// EmailConfig contains configuration for an email channel.
type EmailConfig struct {
	SMTPHost    string
	SMTPPort    int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	Recipients  []string
	CC          []string
	UseTLS      bool
	SkipVerify  bool
	IncludeLogs bool
	PoolSize    int
	ConnTimeout time.Duration
}

// smtpClient wraps an SMTP connection with connection pooling.
type smtpClient struct {
	conn     *smtp.Client
	host     string
	port     int
	username string
	password string
	useTLS   bool
	skipTLS  bool
	timeout  time.Duration
	mu       sync.Mutex
}

// NewEmailChannel creates a new email notification channel.
func NewEmailChannel(cfg EmailConfig, logger *slog.Logger) (*EmailChannel, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.ConnTimeout <= 0 {
		cfg.ConnTimeout = 30 * time.Second
	}

	// Parse templates
	htmlTmpl, err := template.New("email_html").Parse(emailHTMLTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML template: %w", err)
	}

	plainTmpl, err := template.New("email_plain").Parse(emailPlainTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plain template: %w", err)
	}

	channel := &EmailChannel{
		config: cfg,
		client: &smtpClient{
			host:     cfg.SMTPHost,
			port:     cfg.SMTPPort,
			username: cfg.Username,
			password: cfg.Password,
			useTLS:   cfg.UseTLS,
			skipTLS:  cfg.SkipVerify,
			timeout:  cfg.ConnTimeout,
		},
		logger:    logger.With("channel", "email"),
		maxAge:    5 * time.Minute,
		htmlTmpl:  htmlTmpl,
		plainTmpl: plainTmpl,
	}

	return channel, nil
}

// Type returns the channel type.
func (c *EmailChannel) Type() database.ChannelType {
	return database.ChannelTypeEmail
}

// Validate validates the email configuration.
func (c *EmailChannel) Validate() error {
	if c.config.SMTPHost == "" {
		return fmt.Errorf("SMTP host is required")
	}
	if c.config.SMTPPort <= 0 {
		return fmt.Errorf("SMTP port is required")
	}
	if c.config.FromAddress == "" {
		return fmt.Errorf("from address is required")
	}
	if len(c.config.Recipients) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	return nil
}

// Send sends an email notification.
func (c *EmailChannel) Send(ctx context.Context, notification *Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Format email content
	subject := c.formatSubject(notification)
	htmlBody, err := c.formatHTML(notification)
	if err != nil {
		return fmt.Errorf("failed to format HTML body: %w", err)
	}
	plainBody, err := c.formatPlain(notification)
	if err != nil {
		return fmt.Errorf("failed to format plain body: %w", err)
	}

	// Build email message
	msg := c.buildMessage(subject, htmlBody, plainBody)

	// Send with retry
	var lastErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := c.sendEmail(ctx, msg)
		if err == nil {
			c.logger.Debug("email notification sent",
				"notification_type", notification.Type,
				"recipients", len(c.config.Recipients),
			)
			return nil
		}

		lastErr = err
		c.logger.Warn("email send failed, retrying",
			"attempt", attempt+1,
			"error", err,
		)

		// Reset connection on failure
		c.client.close()
	}

	return lastErr
}

// sendEmail sends the email using the SMTP client.
func (c *EmailChannel) sendEmail(ctx context.Context, msg []byte) error {
	// Get or create connection
	client, err := c.client.getConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}

	// Set sender
	if err := client.Mail(c.config.FromAddress); err != nil {
		c.client.close()
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, rcpt := range c.config.Recipients {
		if err := client.Rcpt(rcpt); err != nil {
			c.client.close()
			return fmt.Errorf("failed to set recipient %s: %w", rcpt, err)
		}
	}

	// Set CC recipients
	for _, cc := range c.config.CC {
		if err := client.Rcpt(cc); err != nil {
			c.client.close()
			return fmt.Errorf("failed to set CC recipient %s: %w", cc, err)
		}
	}

	// Send data
	w, err := client.Data()
	if err != nil {
		c.client.close()
		return fmt.Errorf("failed to create data writer: %w", err)
	}

	_, err = w.Write(msg)
	if err != nil {
		c.client.close()
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = w.Close()
	if err != nil {
		c.client.close()
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	c.lastUsed = time.Now()
	return nil
}

// getConnection returns an existing connection or creates a new one.
func (s *smtpClient) getConnection() (*smtp.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		// Test connection with NOOP
		if err := s.conn.Noop(); err == nil {
			return s.conn, nil
		}
		s.conn.Close()
		s.conn = nil
	}

	// Create new connection
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	dialer := &net.Dialer{Timeout: s.timeout}

	var conn net.Conn
	var err error

	if s.useTLS {
		tlsConfig := &tls.Config{
			ServerName:         s.host,
			InsecureSkipVerify: s.skipTLS,
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to dial SMTP server: %w", err)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create SMTP client: %w", err)
	}

	// STARTTLS if not using direct TLS
	if !s.useTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName:         s.host,
				InsecureSkipVerify: s.skipTLS,
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				client.Close()
				return nil, fmt.Errorf("failed to start TLS: %w", err)
			}
		}
	}

	// Authenticate if credentials provided
	if s.username != "" && s.password != "" {
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := client.Auth(auth); err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	s.conn = client
	return client, nil
}

// close closes the SMTP connection.
func (s *smtpClient) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		s.conn.Quit()
		s.conn = nil
	}
}

// formatSubject creates the email subject line.
func (c *EmailChannel) formatSubject(notification *Notification) string {
	prefix := c.getSubjectPrefix(notification.Type)
	return fmt.Sprintf("%s %s - %s", prefix, notification.ServiceName, notification.Title)
}

// getSubjectPrefix returns the appropriate subject prefix.
func (c *EmailChannel) getSubjectPrefix(notificationType NotificationType) string {
	switch notificationType {
	case NotificationTypeRunPassed:
		return "[PASS]"
	case NotificationTypeRunFailed:
		return "[FAIL]"
	case NotificationTypeRunError:
		return "[ERROR]"
	case NotificationTypeRunTimeout:
		return "[TIMEOUT]"
	case NotificationTypeRunRecovered:
		return "[RECOVERED]"
	case NotificationTypeFlakyDetected:
		return "[FLAKY]"
	case NotificationTypeRunStarted:
		return "[STARTED]"
	default:
		return "[INFO]"
	}
}

// emailTemplateData contains data for email templates.
type emailTemplateData struct {
	Title       string
	Message     string
	ServiceName string
	ServiceID   string
	RunID       string
	URL         string
	Summary     *RunSummary
	StatusColor string
	CreatedAt   string
	Year        int
}

// formatHTML formats the notification as HTML email.
func (c *EmailChannel) formatHTML(notification *Notification) (string, error) {
	data := c.getTemplateData(notification)

	var buf bytes.Buffer
	if err := c.htmlTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// formatPlain formats the notification as plain text.
func (c *EmailChannel) formatPlain(notification *Notification) (string, error) {
	data := c.getTemplateData(notification)

	var buf bytes.Buffer
	if err := c.plainTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// getTemplateData prepares template data from a notification.
func (c *EmailChannel) getTemplateData(notification *Notification) emailTemplateData {
	data := emailTemplateData{
		Title:       notification.Title,
		Message:     notification.Message,
		ServiceName: notification.ServiceName,
		URL:         notification.URL,
		Summary:     notification.Summary,
		StatusColor: c.getStatusColor(notification.Type),
		CreatedAt:   notification.CreatedAt.Format("2006-01-02 15:04:05 UTC"),
		Year:        time.Now().Year(),
	}

	if notification.ServiceID != nil {
		data.ServiceID = notification.ServiceID.String()
	}
	if notification.RunID != nil {
		data.RunID = notification.RunID.String()
	}

	return data
}

// getStatusColor returns the color for the notification status.
func (c *EmailChannel) getStatusColor(notificationType NotificationType) string {
	switch notificationType {
	case NotificationTypeRunPassed, NotificationTypeRunRecovered:
		return "#36a64f"
	case NotificationTypeRunFailed, NotificationTypeRunError, NotificationTypeRunTimeout:
		return "#dc3545"
	case NotificationTypeFlakyDetected, NotificationTypeTestQuarantined:
		return "#ffc107"
	default:
		return "#17a2b8"
	}
}

// buildMessage builds the complete MIME message.
func (c *EmailChannel) buildMessage(subject, htmlBody, plainBody string) []byte {
	var msg bytes.Buffer

	boundary := "----=_ConductorBoundary_" + fmt.Sprintf("%d", time.Now().UnixNano())

	// Headers
	from := c.config.FromAddress
	if c.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", c.config.FromName, c.config.FromAddress)
	}

	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(c.config.Recipients, ", ")))
	if len(c.config.CC) > 0 {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(c.config.CC, ", ")))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	// Plain text part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(plainBody)
	msg.WriteString("\r\n")

	// HTML part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	msg.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n")

	// End boundary
	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return msg.Bytes()
}

// Close closes the email channel and releases resources.
func (c *EmailChannel) Close() error {
	c.client.close()
	return nil
}
