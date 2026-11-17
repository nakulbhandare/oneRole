package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

// MailerConfig holds outgoing mail transport settings.
type MailerConfig struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUser     string `json:"smtp_user"`
	SMTPPass     string `json:"smtp_pass,omitempty"`
	SMTPFrom     string `json:"smtp_from"`
	ResendAPIKey string `json:"resend_api_key,omitempty"`
	SendGridKey  string `json:"sendgrid_api_key,omitempty"`
}

// LoadMailerConfigFromEnv retrieves mailer settings from environment variables.
func LoadMailerConfigFromEnv() MailerConfig {
	port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if port <= 0 {
		port = 587
	}
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		if u := os.Getenv("SMTP_USER"); u != "" && strings.Contains(u, "@") {
			from = fmt.Sprintf("oneRole Applications <%s>", u)
		} else {
			from = "oneRole Applications <notifications@onerole.dev>"
		}
	}
	return MailerConfig{
		SMTPHost:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:     port,
		SMTPUser:     strings.TrimSpace(os.Getenv("SMTP_USER")),
		SMTPPass:     strings.TrimSpace(os.Getenv("SMTP_PASS")),
		SMTPFrom:     strings.TrimSpace(from),
		ResendAPIKey: strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		SendGridKey:  strings.TrimSpace(os.Getenv("SENDGRID_API_KEY")),
	}
}

// EmailPayload contains the details of an email to be sent.
type EmailPayload struct {
	Recipient     string `json:"recipient"`
	Subject       string `json:"subject"`
	TextBody      string `json:"text_body"`
	HTMLBody      string `json:"html_body,omitempty"`
	ApplicationID string `json:"application_id,omitempty"`
	ReceiptID     string `json:"receipt_id,omitempty"`
	CompanyName   string `json:"company_name,omitempty"`
	JobTitle      string `json:"job_title,omitempty"`
}

// EmailDeliveryResult details the outcome of an email dispatch attempt.
type EmailDeliveryResult struct {
	Success     bool      `json:"success"`
	Provider    string    `json:"provider"`
	DeliveredAt time.Time `json:"delivered_at"`
	Recipient   string    `json:"recipient"`
	MessageID   string    `json:"message_id,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// Mailer defines the outbound email dispatch contract.
type Mailer interface {
	SendEmail(ctx context.Context, payload EmailPayload) (*EmailDeliveryResult, error)
	GetConfigStatus() map[string]any
}

// DefaultMailer implements Mailer with SMTP, Resend, and SendGrid support.
type DefaultMailer struct {
	cfg        MailerConfig
	httpClient *http.Client
}

// NewDefaultMailer instantiates a DefaultMailer configured from environment.
func NewDefaultMailer() *DefaultMailer {
	return NewMailerWithConfig(LoadMailerConfigFromEnv())
}

// NewMailerWithConfig instantiates a DefaultMailer with an explicit configuration.
func NewMailerWithConfig(cfg MailerConfig) *DefaultMailer {
	return &DefaultMailer{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// GetConfigStatus returns non-sensitive status about the active outbound mail transport.
func (m *DefaultMailer) GetConfigStatus() map[string]any {
	hasSMTP := m.cfg.SMTPHost != ""
	hasResend := m.cfg.ResendAPIKey != ""
	hasSendGrid := m.cfg.SendGridKey != ""

	activeProvider := "none"
	if hasResend {
		activeProvider = "resend_api"
	} else if hasSendGrid {
		activeProvider = "sendgrid_api"
	} else if hasSMTP {
		activeProvider = fmt.Sprintf("smtp (%s:%d)", m.cfg.SMTPHost, m.cfg.SMTPPort)
	}

	return map[string]any{
		"active_provider":  activeProvider,
		"smtp_configured":  hasSMTP,
		"smtp_host":        m.cfg.SMTPHost,
		"smtp_port":        m.cfg.SMTPPort,
		"smtp_user":        m.cfg.SMTPUser,
		"smtp_from":        m.cfg.SMTPFrom,
		"resend_enabled":   hasResend,
		"sendgrid_enabled": hasSendGrid,
		"is_ready":         hasSMTP || hasResend || hasSendGrid,
	}
}

// SendEmail delivers the message to the recipient using the highest-priority configured transport.
func (m *DefaultMailer) SendEmail(ctx context.Context, payload EmailPayload) (*EmailDeliveryResult, error) {
	recipient := strings.TrimSpace(payload.Recipient)
	if recipient == "" {
		recipient = "bhandarenakul11@gmail.com"
	}

	// 1. Prioritize Resend API if key is present
	if m.cfg.ResendAPIKey != "" {
		return m.sendViaResend(ctx, recipient, payload)
	}

	// 2. Prioritize SendGrid API if key is present
	if m.cfg.SendGridKey != "" {
		return m.sendViaSendGrid(ctx, recipient, payload)
	}

	// 3. Use SMTP if host is set
	if m.cfg.SMTPHost != "" {
		return m.sendViaSMTP(ctx, recipient, payload)
	}

	// 4. No outbound mail provider configured
	err := errors.New("outbound mail transport unconfigured: please provide SMTP_HOST/SMTP_PASS in .env or configure RESEND_API_KEY / SENDGRID_API_KEY")
	return &EmailDeliveryResult{
		Success:     false,
		Provider:    "unconfigured",
		DeliveredAt: time.Now().UTC(),
		Recipient:   recipient,
		Error:       err.Error(),
	}, err
}

func (m *DefaultMailer) sendViaResend(ctx context.Context, recipient string, payload EmailPayload) (*EmailDeliveryResult, error) {
	reqBody := map[string]any{
		"from":    m.cfg.SMTPFrom,
		"to":      []string{recipient},
		"subject": payload.Subject,
		"text":    payload.TextBody,
	}
	if payload.HTMLBody != "" {
		reqBody["html"] = payload.HTMLBody
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Resend payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create Resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return &EmailDeliveryResult{
			Success:     false,
			Provider:    "resend_api",
			DeliveredAt: time.Now().UTC(),
			Recipient:   recipient,
			Error:       err.Error(),
		}, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := fmt.Sprintf("Resend API rejected request (HTTP %d): %s", resp.StatusCode, string(respBytes))
		return &EmailDeliveryResult{
			Success:     false,
			Provider:    "resend_api",
			DeliveredAt: time.Now().UTC(),
			Recipient:   recipient,
			Error:       errMsg,
		}, errors.New(errMsg)
	}

	var resendResp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(respBytes, &resendResp)

	return &EmailDeliveryResult{
		Success:     true,
		Provider:    "resend_api",
		DeliveredAt: time.Now().UTC(),
		Recipient:   recipient,
		MessageID:   resendResp.ID,
	}, nil
}

func (m *DefaultMailer) sendViaSendGrid(ctx context.Context, recipient string, payload EmailPayload) (*EmailDeliveryResult, error) {
	reqBody := map[string]any{
		"personalizations": []map[string]any{
			{
				"to": []map[string]string{
					{"email": recipient},
				},
			},
		},
		"from": map[string]string{
			"email": cleanEmailOnly(m.cfg.SMTPFrom),
			"name":  "oneRole Applications",
		},
		"subject": payload.Subject,
		"content": []map[string]string{
			{
				"type":  "text/plain",
				"value": payload.TextBody,
			},
		},
	}
	if payload.HTMLBody != "" {
		reqBody["content"] = append(reqBody["content"].([]map[string]string), map[string]string{
			"type":  "text/html",
			"value": payload.HTMLBody,
		})
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to encode SendGrid payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.sendgrid.com/v3/mail/send", bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create SendGrid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.SendGridKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return &EmailDeliveryResult{
			Success:     false,
			Provider:    "sendgrid_api",
			DeliveredAt: time.Now().UTC(),
			Recipient:   recipient,
			Error:       err.Error(),
		}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("SendGrid API rejected request (HTTP %d): %s", resp.StatusCode, string(respBytes))
		return &EmailDeliveryResult{
			Success:     false,
			Provider:    "sendgrid_api",
			DeliveredAt: time.Now().UTC(),
			Recipient:   recipient,
			Error:       errMsg,
		}, errors.New(errMsg)
	}

	return &EmailDeliveryResult{
		Success:     true,
		Provider:    "sendgrid_api",
		DeliveredAt: time.Now().UTC(),
		Recipient:   recipient,
		MessageID:   resp.Header.Get("X-Message-Id"),
	}, nil
}

func (m *DefaultMailer) sendViaSMTP(ctx context.Context, recipient string, payload EmailPayload) (*EmailDeliveryResult, error) {
	addr := fmt.Sprintf("%s:%d", m.cfg.SMTPHost, m.cfg.SMTPPort)
	fromEmail := cleanEmailOnly(m.cfg.SMTPFrom)
	now := time.Now().UTC()
	msgID := fmt.Sprintf("<%d.%s@onerole.dev>", now.UnixNano(), payload.ReceiptID)

	var msgBuffer bytes.Buffer
	msgBuffer.WriteString(fmt.Sprintf("From: %s\r\n", m.cfg.SMTPFrom))
	msgBuffer.WriteString(fmt.Sprintf("To: %s\r\n", recipient))
	msgBuffer.WriteString(fmt.Sprintf("Subject: %s\r\n", payload.Subject))
	msgBuffer.WriteString(fmt.Sprintf("Date: %s\r\n", now.Format(time.RFC1123Z)))
	msgBuffer.WriteString(fmt.Sprintf("Message-ID: %s\r\n", msgID))
	msgBuffer.WriteString("MIME-Version: 1.0\r\n")

	if payload.HTMLBody != "" {
		boundary := fmt.Sprintf("boundary_%d", now.UnixNano())
		msgBuffer.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))
		msgBuffer.WriteString(fmt.Sprintf("--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n\r\n", boundary, payload.TextBody))
		msgBuffer.WriteString(fmt.Sprintf("--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n\r\n", boundary, payload.HTMLBody))
		msgBuffer.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		msgBuffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		msgBuffer.WriteString(payload.TextBody)
	}

	var auth smtp.Auth
	if m.cfg.SMTPUser != "" && m.cfg.SMTPPass != "" {
		auth = smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	}

	var client *smtp.Client
	var err error

	// Connect based on port: 465 is direct TLS, 587/25 uses STARTTLS
	if m.cfg.SMTPPort == 465 {
		tlsConfig := &tls.Config{
			ServerName: m.cfg.SMTPHost,
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, dialErr := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if dialErr != nil {
			return &EmailDeliveryResult{
				Success:     false,
				Provider:    "smtp_tls",
				DeliveredAt: now,
				Recipient:   recipient,
				Error:       dialErr.Error(),
			}, fmt.Errorf("SMTP TLS dial failed to %s: %w", addr, dialErr)
		}
		client, err = smtp.NewClient(conn, m.cfg.SMTPHost)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("SMTP client initialization failed: %w", err)
		}
	} else {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
		if dialErr != nil {
			return &EmailDeliveryResult{
				Success:     false,
				Provider:    "smtp_starttls",
				DeliveredAt: now,
				Recipient:   recipient,
				Error:       dialErr.Error(),
			}, fmt.Errorf("SMTP connection failed to %s: %w", addr, dialErr)
		}
		client, err = smtp.NewClient(conn, m.cfg.SMTPHost)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("SMTP client initialization failed: %w", err)
		}

		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName: m.cfg.SMTPHost,
			}
			if err = client.StartTLS(tlsConfig); err != nil {
				_ = client.Close()
				return &EmailDeliveryResult{
					Success:     false,
					Provider:    "smtp_starttls",
					DeliveredAt: now,
					Recipient:   recipient,
					Error:       err.Error(),
				}, fmt.Errorf("STARTTLS negotiation failed: %w", err)
			}
		}
	}
	defer client.Quit()

	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return &EmailDeliveryResult{
				Success:     false,
				Provider:    "smtp_auth",
				DeliveredAt: now,
				Recipient:   recipient,
				Error:       err.Error(),
			}, fmt.Errorf("SMTP authentication failed for user %s: %w", m.cfg.SMTPUser, err)
		}
	}

	if err = client.Mail(fromEmail); err != nil {
		return nil, fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	if err = client.Rcpt(recipient); err != nil {
		return nil, fmt.Errorf("SMTP RCPT TO %s failed: %w", recipient, err)
	}

	w, err := client.Data()
	if err != nil {
		return nil, fmt.Errorf("SMTP DATA command failed: %w", err)
	}
	if _, err = w.Write(msgBuffer.Bytes()); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("failed to write email body: %w", err)
	}
	if err = w.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize email body: %w", err)
	}

	return &EmailDeliveryResult{
		Success:     true,
		Provider:    fmt.Sprintf("smtp (%s)", m.cfg.SMTPHost),
		DeliveredAt: now,
		Recipient:   recipient,
		MessageID:   msgID,
	}, nil
}

func cleanEmailOnly(input string) string {
	input = strings.TrimSpace(input)
	if open := strings.Index(input, "<"); open != -1 {
		if close := strings.LastIndex(input, ">"); close > open {
			return strings.TrimSpace(input[open+1 : close])
		}
	}
	return input
}

// GenerateConfirmationEmailContent creates formatted plain text and HTML confirmation emails.
func GenerateConfirmationEmailContent(jobTitle, companyName, appID, receiptID string, submitTime time.Time) (string, string) {
	textBody := fmt.Sprintf(`Hello Nakul,

Your job application for %s at %s has been successfully submitted!

Details of your submission:
- Position: %s
- Company: %s
- Application ID: %s
- Receipt ID: %s
- Status: Submitted & Verified
- Submission Timestamp: %s

You can view your tailored resume, submission receipt, and tracking timeline anytime in the oneRole dashboard:
http://localhost:8082

Best regards,
The oneRole Team
https://onerole.dev
`, jobTitle, companyName, jobTitle, companyName, appID, receiptID, submitTime.Format(time.RFC3339))

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Application Submitted - oneRole</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; background-color: #f8fafc; margin: 0; padding: 24px; color: #1e293b; }
    .card { max-width: 580px; margin: 0 auto; background: #ffffff; border-radius: 12px; border: 1px solid #e2e8f0; overflow: hidden; box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05); }
    .header { background: #064e3b; padding: 24px 32px; color: #ffffff; }
    .header h1 { margin: 0; font-size: 20px; font-weight: 700; letter-spacing: -0.02em; }
    .content { padding: 32px; }
    .badge { display: inline-block; background: #ecfdf5; color: #047857; font-weight: 600; font-size: 12px; padding: 4px 10px; border-radius: 9999px; border: 1px solid #a7f3d0; margin-bottom: 16px; }
    .details-table { width: 100%%; border-collapse: collapse; margin: 20px 0; background: #f8fafc; border-radius: 8px; overflow: hidden; }
    .details-table td { padding: 12px 16px; border-bottom: 1px solid #e2e8f0; font-size: 14px; }
    .details-table td.label { font-weight: 600; color: #64748b; width: 35%%; }
    .details-table td.value { font-weight: 500; color: #0f172a; word-break: break-all; }
    .cta-btn { display: inline-block; background: #059669; color: #ffffff !important; text-decoration: none; font-weight: 600; font-size: 14px; padding: 12px 24px; border-radius: 8px; margin-top: 16px; }
    .footer { padding: 20px 32px; background: #f1f5f9; text-align: center; font-size: 12px; color: #64748b; border-top: 1px solid #e2e8f0; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <h1>oneRole • Application Confirmed</h1>
    </div>
    <div class="content">
      <div class="badge">✓ Successfully Submitted</div>
      <p style="font-size: 16px; line-height: 1.5; margin-top: 0;">
        Hello Nakul, your application for <strong>%s</strong> at <strong>%s</strong> was submitted and verified by the oneRole autonomous engine.
      </p>
      
      <table class="details-table">
        <tr>
          <td class="label">Position</td>
          <td class="value"><strong>%s</strong></td>
        </tr>
        <tr>
          <td class="label">Company</td>
          <td class="value"><strong>%s</strong></td>
        </tr>
        <tr>
          <td class="label">Application ID</td>
          <td class="value"><code>%s</code></td>
        </tr>
        <tr>
          <td class="label">Receipt ID</td>
          <td class="value"><code>%s</code></td>
        </tr>
        <tr>
          <td class="label">Timestamp</td>
          <td class="value">%s</td>
        </tr>
      </table>

      <div style="text-align: center; margin: 24px 0 8px 0;">
        <a href="http://localhost:8082" class="cta-btn">View in Application Tracker →</a>
      </div>
    </div>
    <div class="footer">
      This is an automated notification from oneRole Autonomous Career Infrastructure.<br/>
      © %d oneRole. All rights reserved.
    </div>
  </div>
</body>
</html>`, jobTitle, companyName, jobTitle, companyName, appID, receiptID, submitTime.Format("Jan 02, 2006 15:04:05 UTC"), time.Now().Year())

	return textBody, htmlBody
}
