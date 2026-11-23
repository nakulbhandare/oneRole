package emailgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/oneRole/connectors/email_otp"
	"github.com/oneRole/internal/emailidentity"
	"github.com/oneRole/models"
	"github.com/oneRole/service"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	rxReceiptHash = regexp.MustCompile(`(?:^|\s)#([A-Z0-9][A-Za-z0-9_-]{4,25})\b`)
	rxReceiptRef  = regexp.MustCompile(`(?i)(?:confirmation|application\s+reference|reference|submission)(?:\s+(?:number|id|code|ref))?[:\s]+(?:is\s+)?#?([A-Z0-9][A-Za-z0-9_-]{4,39})`)
	rxActionURL   = regexp.MustCompile(`https?://[^\s"'<>]+(?:verify|confirm|activate|token|code|calendly|zoom\.us|codesignal|hackerrank)[^\s"'<>]*`)
)

type GatewayConfig struct {
	WebhookSecret string
	ForwardDomain string
}

type Gateway struct {
	db       *mongo.Database
	identity *emailidentity.Service
	mailer   service.Mailer
	otpReg   *email_otp.Registry
	config   GatewayConfig
}

func NewGateway(db *mongo.Database, identitySvc *emailidentity.Service, mailer service.Mailer, cfg GatewayConfig) *Gateway {
	gw := &Gateway{
		db:       db,
		identity: identitySvc,
		mailer:   mailer,
		otpReg:   email_otp.GetRegistry(),
		config:   cfg,
	}
	gw.ensureIndexes(context.Background())
	return gw
}

func (gw *Gateway) ensureIndexes(ctx context.Context) {
	if gw.db == nil {
		return
	}
	// 1. email_messages indexes
	msgColl := gw.db.Collection("email_messages")
	_, _ = msgColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "message_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_message_id"),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "received_at", Value: -1}},
			Options: options.Index().SetName("idx_user_received"),
		},
		{
			Keys:    bson.D{{Key: "recipient_email", Value: 1}},
			Options: options.Index().SetName("idx_recipient_email"),
		},
		{
			Keys:    bson.D{{Key: "correlated_application_id", Value: 1}},
			Options: options.Index().SetName("idx_correlated_app"),
		},
	})

	// 2. email_otp_challenges indexes
	otpColl := gw.db.Collection("email_otp_challenges")
	_, _ = otpColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("idx_user_otp_expiry"),
		},
		{
			Keys:    bson.D{{Key: "recipient_email", Value: 1}, {Key: "employer_domain", Value: 1}},
			Options: options.Index().SetName("idx_recipient_employer"),
		},
	})

	// 3. email_forwarding_events index
	fwdColl := gw.db.Collection("email_forwarding_events")
	_, _ = fwdColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email_id", Value: 1}, {Key: "attempted_at", Value: -1}},
		Options: options.Index().SetName("idx_forward_email"),
	})
}

// InboundWebhookPayload models an incoming webhook from an email provider.
type InboundWebhookPayload struct {
	To          string            `json:"to"`
	From        string            `json:"from"`
	Subject     string            `json:"subject"`
	Text        string            `json:"text"`
	HTML        string            `json:"html"`
	MessageID   string            `json:"message_id,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Signature   string            `json:"signature,omitempty"`
	Timestamp   int64             `json:"timestamp,omitempty"`
}

// ProcessInbound handles the entire incoming email lifecycle.
func (gw *Gateway) ProcessInbound(ctx context.Context, payload InboundWebhookPayload) (*models.EmailMessage, error) {
	rawTo := strings.TrimSpace(payload.To)
	if rawTo == "" {
		return nil, errors.New("recipient address ('to') is required")
	}

	cleanTo := extractBareEmail(rawTo)
	cleanFrom := extractBareEmail(payload.From)
	now := time.Now().UTC()

	// 1. Recipient Resolution
	var ident *models.UserEmailIdentity
	var err error
	if gw.identity != nil {
		ident, err = gw.identity.ResolveRecipient(ctx, cleanTo)
	}
	if err != nil || ident == nil {
		return nil, fmt.Errorf("recipient resolution failed for %s: %w", cleanTo, err)
	}

	// 2. Message Deduplication
	msgID := payload.MessageID
	if msgID == "" {
		// Generate deterministic fingerprint from sender, recipient, subject, and minute
		hasher := sha256.New()
		hasher.Write([]byte(fmt.Sprintf("%s:%s:%s:%d", cleanFrom, cleanTo, payload.Subject, now.Unix()/60)))
		msgID = fmt.Sprintf("msg-%s", hex.EncodeToString(hasher.Sum(nil))[:24])
	}

	if gw.db != nil {
		var existing models.EmailMessage
		if dErr := gw.db.Collection("email_messages").FindOne(ctx, bson.M{"message_id": msgID}).Decode(&existing); dErr == nil {
			return &existing, nil // Idempotent return
		}
	}

	// 3. Extraction & Classification
	employerDomain := extractDomain(cleanFrom)
	employerName := extractEmployerName(payload.From, payload.Subject)
	category := classifyCategory(payload.Subject, payload.Text, payload.HTML)
	receiptID := extractReceipt(payload.Subject, payload.Text, payload.HTML)
	actionURL := rxActionURL.FindString(fmt.Sprintf("%s %s", payload.Text, payload.HTML))

	emailID := fmt.Sprintf("em-%d-%s", now.UnixNano(), msgID[:8])
	msgRecord := models.EmailMessage{
		ID:                 emailID,
		MessageID:          msgID,
		UserID:             ident.UserID,
		TenantID:           ident.TenantID,
		RecipientEmail:     cleanTo,
		SenderEmail:        cleanFrom,
		SenderName:         payload.From,
		EmployerDomain:     employerDomain,
		EmployerName:       employerName,
		Subject:            payload.Subject,
		Snippet:            buildSnippet(payload.Text, payload.HTML),
		Category:           category,
		VerifiedReceiptID:  receiptID,
		ExtractedActionURL: actionURL,
		ForwardStatus:      "pending",
		ForwardedTo:        ident.PersonalForwardingEmail,
		Headers:            payload.Headers,
		ReceivedAt:         now,
		CreatedAt:          now,
	}

	// 4. PATH 1: HIGH-PRIORITY OTP PROCESSING (Non-blocking for forwarding)
	// Immediately deliver to OTP Registry so browser worker wakes up instantly
	otpPayload := gw.otpReg.DeliverIncoming(cleanTo, employerDomain, payload.Subject, fmt.Sprintf("%s\n%s", payload.Text, payload.HTML))
	if otpPayload != nil && (otpPayload.Code != "" || otpPayload.MagicLink != "") {
		msgRecord.ContainsOTP = true
		msgRecord.Category = models.CategoryOTP

		// Persist short-lived OTP challenge for auditing (zero exposure of raw code)
		if gw.db != nil {
			hasher := sha256.New()
			hasher.Write([]byte(otpPayload.Code + ":onerole-salt"))
			codeHash := hex.EncodeToString(hasher.Sum(nil))

			challenge := models.EmailOTPChallenge{
				ID:             fmt.Sprintf("otp-chal-%d", now.UnixNano()),
				UserID:         ident.UserID,
				EmployerDomain: employerDomain,
				RecipientEmail: cleanTo,
				CodeHash:       codeHash,
				MagicLink:      otpPayload.MagicLink,
				Consumed:       false,
				ExpiresAt:      now.Add(10 * time.Minute),
				CreatedAt:      now,
			}
			_, _ = gw.db.Collection("email_otp_challenges").InsertOne(ctx, challenge)
		}
	}

	// 5. PATH 2: APPLICATION CORRELATION
	// Correlate with active applications for this user and employer
	if gw.db != nil {
		var activeApp struct {
			ID     string `bson:"_id"`
			Status string `bson:"status"`
		}
		compRegex := bson.M{"$regex": regexp.QuoteMeta(employerName), "$options": "i"}
		filter := bson.M{
			"user_id": ident.UserID,
			"$or": []bson.M{
				{"company_name": compRegex},
				{"company": compRegex},
			},
		}
		if aErr := gw.db.Collection("applications").FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})).Decode(&activeApp); aErr == nil {
			msgRecord.CorrelatedApplicationID = activeApp.ID

			// If this is an employer confirmation with verified receipt, update application status!
			if (category == models.CategoryConfirmation || category == models.CategoryApplicationReceived || receiptID != "") && activeApp.Status != "SUBMITTED_CONFIRMED" {
				updateFields := bson.M{
					"status":                          "SUBMITTED_CONFIRMED",
					"canonical_status":                "SUBMITTED_CONFIRMED",
					"confirmation_email_received_at": now,
					"email_verification_status":       "verified_employer_inbox",
					"email_message_id":                msgID,
					"updated_at":                      now,
				}
				if receiptID != "" {
					updateFields["verified_receipt_id"] = receiptID
				}
				_, _ = gw.db.Collection("applications").UpdateOne(ctx, bson.M{"_id": activeApp.ID}, bson.M{"$set": updateFields})

				// Record timeline event
				_, _ = gw.db.Collection("application_events").InsertOne(ctx, bson.M{
					"_id":            fmt.Sprintf("evt-conf-%d", now.UnixNano()),
					"tenant_id":      ident.TenantID,
					"application_id": activeApp.ID,
					"event_type":     "application.submitted_confirmed",
					"from_status":    activeApp.Status,
					"to_status":      "SUBMITTED_CONFIRMED",
					"actor_type":     "email_gateway",
					"actor_id":       ident.UserID,
					"occurred_at":    now,
					"metadata": bson.M{
						"receipt_id": receiptID,
						"from":       cleanFrom,
						"subject":    payload.Subject,
					},
				})
			}
		}
	}

	// 6. Persist email message record
	if gw.db != nil {
		_, _ = gw.db.Collection("email_messages").InsertOne(ctx, msgRecord)
		_, _ = gw.db.Collection("email_processing_events").InsertOne(ctx, models.EmailProcessingEvent{
			ID:         fmt.Sprintf("proc-evt-%d", now.UnixNano()),
			EmailID:    emailID,
			UserID:     ident.UserID,
			EventType:  "email.received_and_processed",
			Details: map[string]interface{}{
				"category":       category,
				"has_otp":        msgRecord.ContainsOTP,
				"receipt_id":     receiptID,
				"correlated_app": msgRecord.CorrelatedApplicationID,
			},
			OccurredAt: now,
		})
	}

	// 7. PATH 3: ASYNCHRONOUS AUTOMATIC FORWARDING TO PERSONAL INBOX
	// Never blocks the HTTP gateway or OTP fulfillment
	go gw.dispatchForwarding(ident.UserID, emailID, ident.PersonalForwardingEmail, payload, msgRecord)

	return &msgRecord, nil
}

// dispatchForwarding executes asynchronous outbound delivery to personal inbox.
func (gw *Gateway) dispatchForwarding(userID, emailID, destination string, raw InboundWebhookPayload, meta models.EmailMessage) {
	if destination == "" || gw.mailer == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	now := time.Now().UTC()
	fwdSubject := fmt.Sprintf("[OneRole Forwarded] %s", raw.Subject)
	if meta.ContainsOTP {
		fwdSubject = fmt.Sprintf("🔑 [OneRole OTP] %s", raw.Subject)
	}

	fwdHTML := fmt.Sprintf(`
<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; max-width: 680px; margin: 0 auto; padding: 16px; background-color: #f8fafc; border-radius: 8px; border: 1px solid #e2e8f0; margin-bottom: 20px;">
  <div style="font-size: 13px; font-weight: 700; color: #0f172a; margin-bottom: 4px;">OneRole Automated Email Gateway</div>
  <div style="font-size: 12px; color: #475569; margin-bottom: 2px;">Employer: <strong>%s</strong> &bull; Category: <span style="display:inline-block; padding: 2px 8px; border-radius: 9999px; background: #e0f2fe; color: #0369a1; font-weight: 600; font-size: 11px;">%s</span></div>
  <div style="font-size: 12px; color: #475569;">Received via: <strong>%s</strong> &rarr; Forwarded to: <strong>%s</strong></div>
  <div style="font-size: 12px; color: #64748b;">Original Sender: <strong>%s</strong></div>
</div>
%s
`, meta.EmployerName, meta.Category, meta.RecipientEmail, destination, raw.From, raw.HTML)

	res, err := gw.mailer.SendEmail(ctx, service.EmailPayload{
		Recipient:   destination,
		Subject:     fwdSubject,
		HTMLBody:    fwdHTML,
		TextBody:    raw.Text,
		CompanyName: meta.EmployerName,
		ReceiptID:   meta.VerifiedReceiptID,
	})

	fwdStatus := "forwarded"
	deliveryErr := ""
	if err != nil || (res != nil && !res.Success) {
		fwdStatus = "failed"
		if err != nil {
			deliveryErr = err.Error()
		} else if res != nil {
			deliveryErr = res.Error
		}
	}

	if gw.db != nil {
		_, _ = gw.db.Collection("email_messages").UpdateOne(ctx,
			bson.M{"_id": emailID},
			bson.M{"$set": bson.M{
				"forward_status": fwdStatus,
				"forwarded_to":   destination,
				"forwarded_at":   now,
				"forward_error":  deliveryErr,
			}},
		)

		_, _ = gw.db.Collection("email_forwarding_events").InsertOne(ctx, models.EmailForwardingEvent{
			ID:            fmt.Sprintf("fwd-evt-%d", now.UnixNano()),
			EmailID:       emailID,
			UserID:        userID,
			Destination:   destination,
			AttemptNumber: 1,
			Status:        fwdStatus,
			DeliveryError: deliveryErr,
			ProviderName:  "smtp_forwarder",
			AttemptedAt:   now,
		})

		// Increment forwarded counter on identity
		_, _ = gw.db.Collection(emailidentity.CollectionIdentities).UpdateOne(ctx,
			bson.M{"user_id": userID},
			bson.M{
				"$inc": bson.M{"total_emails_received": 1, "total_emails_forwarded": 1},
				"$set": bson.M{"updated_at": now},
			},
		)
	}
}

// Helpers

func extractBareEmail(raw string) string {
	parts := strings.Split(raw, "<")
	if len(parts) > 1 {
		clean := strings.TrimRight(parts[1], ">")
		return strings.ToLower(strings.TrimSpace(clean))
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return strings.ToLower(parts[1])
	}
	return ""
}

func extractEmployerName(from, subject string) string {
	parts := strings.Split(from, "<")
	if len(parts) > 1 {
		name := strings.TrimSpace(parts[0])
		name = strings.Trim(name, `"'`)
		name = strings.ReplaceAll(name, " Careers", "")
		name = strings.ReplaceAll(name, " Recruiting", "")
		name = strings.ReplaceAll(name, " Talent", "")
		name = strings.ReplaceAll(name, " Acquisition", "")
		name = strings.ReplaceAll(name, " Team", "")
		name = strings.TrimSpace(name)
		if name != "" {
			return name
		}
	}

	lowerSub := strings.ToLower(subject)
	if idx := strings.Index(lowerSub, "applying to "); idx != -1 {
		rest := subject[idx+len("applying to "):]
		tokens := strings.Split(rest, " - ")
		if len(tokens) > 0 && strings.TrimSpace(tokens[0]) != "" {
			return strings.TrimSpace(tokens[0])
		}
	}
	if idx := strings.Index(lowerSub, "at "); idx != -1 {
		rest := subject[idx+3:]
		tokens := strings.Split(rest, " ")
		if len(tokens) > 0 {
			return strings.Trim(tokens[0], ".,!?")
		}
	}
	return "Employer"
}

func classifyCategory(subject, text, html string) models.EmailCategory {
	combined := strings.ToLower(fmt.Sprintf("%s %s %s", subject, text, html))

	if strings.Contains(combined, "verification code") || strings.Contains(combined, "security code") ||
		strings.Contains(combined, "passcode") || strings.Contains(combined, "one-time password") ||
		strings.Contains(combined, "enter code") {
		return models.CategoryOTP
	}
	if strings.Contains(combined, "interview") || strings.Contains(combined, "schedule a call") || strings.Contains(combined, "phone screen") {
		return models.CategoryInterview
	}
	if strings.Contains(combined, "assessment") || strings.Contains(combined, "codesignal") || strings.Contains(combined, "hackerrank") || strings.Contains(combined, "coding challenge") {
		return models.CategoryAssessment
	}
	if strings.Contains(combined, "unfortunately") || strings.Contains(combined, "not moving forward") || strings.Contains(combined, "other candidates") {
		return models.CategoryRejection
	}
	if strings.Contains(combined, "application submitted") || strings.Contains(combined, "received your application") || strings.Contains(combined, "thank you for applying") {
		return models.CategoryConfirmation
	}
	if strings.Contains(combined, "recruiter") || strings.Contains(combined, "talent partner") {
		return models.CategoryRecruiterDirect
	}
	return models.CategoryGeneral
}

func extractReceipt(subject, text, html string) string {
	combined := fmt.Sprintf("%s\n%s\n%s", subject, text, html)
	if m := rxReceiptHash.FindStringSubmatch(combined); len(m) > 1 {
		val := strings.TrimSpace(m[1])
		if !strings.EqualFold(val, "number") && !strings.EqualFold(val, "reference") {
			return val
		}
	}
	if m := rxReceiptRef.FindStringSubmatch(combined); len(m) > 1 {
		val := strings.TrimSpace(m[1])
		if !strings.EqualFold(val, "number") && !strings.EqualFold(val, "reference") && !strings.EqualFold(val, "application") {
			return val
		}
	}
	return ""
}

func buildSnippet(text, html string) string {
	clean := text
	if strings.TrimSpace(clean) == "" {
		r := regexp.MustCompile(`<[^>]*>`)
		clean = r.ReplaceAllString(html, " ")
	}
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 160 {
		return clean[:157] + "..."
	}
	return clean
}
