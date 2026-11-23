package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// InboundEmailPayload represents an incoming email payload from an Inbound Mail gateway.
type InboundEmailPayload struct {
	To          string            `json:"to"`
	From        string            `json:"from"`
	Subject     string            `json:"subject"`
	TextBody    string            `json:"text_body"`
	HTMLBody    string            `json:"html_body"`
	MessageID   string            `json:"message_id,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ReceivedAt  time.Time         `json:"received_at,omitempty"`
}

// InboundEmailResult details the outcome of processing and routing the inbound email.
type InboundEmailResult struct {
	Success        bool      `json:"success"`
	EmailID        string    `json:"email_id"`
	UserID         string    `json:"user_id"`
	CandidateEmail string    `json:"candidate_email"`
	RoutingEmail   string    `json:"routing_email"`
	Company        string    `json:"company"`
	JobTitle       string    `json:"job_title"`
	Category       string    `json:"category"`
	ForwardedTo    string    `json:"forwarded_to,omitempty"`
	ForwardStatus  string    `json:"forward_status"`
	DeliveryError  string    `json:"delivery_error,omitempty"`
	CorrelatedAppID string   `json:"correlated_application_id,omitempty"`
	VerifiedReceipt string   `json:"verified_receipt_id,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

var (
	rxCalendly    = regexp.MustCompile(`https?://calendly\.com/[^\s"'>]+`)
	rxZoom        = regexp.MustCompile(`https?://[a-zA-Z0-9-]+\.zoom\.us/[^\s"'>]+`)
	rxCodeSig     = regexp.MustCompile(`https?://[^\s"'>]*codesignal\.com/[^\s"'>]+`)
	rxHackerR     = regexp.MustCompile(`https?://[^\s"'>]*hackerrank\.com/[^\s"'>]+`)
	rxTracker     = regexp.MustCompile(`https?://[^\s"'>]+/(?:tracker|status|application)[^\s"'>]*`)
	rxReceiptHash = regexp.MustCompile(`(?:^|\s)#([A-Z0-9][A-Za-z0-9_-]{4,25})\b`)
	rxReceiptRef  = regexp.MustCompile(`(?i)(?:confirmation|application\s+reference|reference|submission)(?:\s+(?:number|id|code|ref))?[:\s]+(?:is\s+)?#?([A-Z0-9][A-Za-z0-9_-]{4,39})`)
)

func extractEmployerReceiptID(subject, text, html string) string {
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

func detectAtsFromEmail(from, subject, body string) string {
	combined := strings.ToLower(from + " " + subject + " " + body)
	if strings.Contains(combined, "greenhouse") || strings.Contains(combined, "gh-mail") {
		return "greenhouse"
	}
	if strings.Contains(combined, "lever.co") || strings.Contains(combined, "hire.lever") {
		return "lever"
	}
	if strings.Contains(combined, "ashbyhq") || strings.Contains(combined, "ashby.io") {
		return "ashby"
	}
	if strings.Contains(combined, "workday") || strings.Contains(combined, "myworkday") {
		return "workday"
	}
	if strings.Contains(combined, "smartrecruiters") {
		return "smartrecruiters"
	}
	return "direct_employer"
}

// GetCandidateRoutingEmail gets or creates the user's unique routing email.
func (s *EnterpriseService) GetCandidateRoutingEmail(ctx context.Context, userID, tenantID string) (string, error) {
	if s.db == nil {
		return GenerateUserRoutingEmail(userID, "candidate", "app"), nil
	}

	var prof struct {
		FirstName         string `bson:"first_name"`
		LastName          string `bson:"last_name"`
		EmailRoutingAlias string `bson:"email_routing_alias"`
		ApplicationEmail  string `bson:"application_email"`
	}

	err := s.db.Collection("candidate_profiles").FindOne(ctx, bson.M{"user_id": userID}).Decode(&prof)
	if err == nil && prof.EmailRoutingAlias != "" {
		return prof.EmailRoutingAlias, nil
	}

	fn := prof.FirstName
	if fn == "" {
		fn = "Nakul"
	}
	ln := prof.LastName
	if ln == "" {
		ln = "Bhandare"
	}

	routingEmail := GenerateUserRoutingEmail(userID, fn, ln)

	// Persist to candidate profile
	_, _ = s.db.Collection("candidate_profiles").UpdateOne(ctx,
		bson.M{"user_id": userID},
		bson.M{"$set": bson.M{
			"email_routing_alias": routingEmail,
			"application_email":   routingEmail,
			"updated_at":          time.Now().UTC(),
		}},
	)

	return routingEmail, nil
}

// ProcessInboundEmail parses an incoming employer email, records it in MongoDB, and auto-forwards to the user.
func (s *EnterpriseService) ProcessInboundEmail(ctx context.Context, payload InboundEmailPayload) (*InboundEmailResult, error) {
	now := time.Now().UTC()
	if payload.ReceivedAt.IsZero() {
		payload.ReceivedAt = now
	}

	// 1. Resolve Target User from Routing Address
	cleanTo := strings.TrimSpace(strings.ToLower(payload.To))
	token := ExtractRoutingToken(cleanTo)

	var targetUserID string
	var personalEmail string
	var tenantID string = "00000000-0000-4000-8000-000000000001"

	if s.db != nil {
		// Try matching by exact alias or routing token
		var pDoc struct {
			UserID            string `bson:"user_id"`
			TenantID          string `bson:"tenant_id"`
			Email             string `bson:"email"`
			EmailRoutingAlias string `bson:"email_routing_alias"`
		}

		filter := bson.M{
			"$or": []bson.M{
				{"email_routing_alias": cleanTo},
				{"application_email": cleanTo},
			},
		}
		if token != "" {
			filter["$or"] = append(filter["$or"].([]bson.M), bson.M{"email_routing_alias": bson.M{"$regex": token}})
		}

		err := s.db.Collection("candidate_profiles").FindOne(ctx, filter).Decode(&pDoc)
		if err == nil && pDoc.UserID != "" {
			targetUserID = pDoc.UserID
			personalEmail = pDoc.Email
			if pDoc.TenantID != "" {
				tenantID = pDoc.TenantID
			}
		} else {
			// Fallback: look up default active candidate user
			var uDoc struct {
				ID       string `bson:"_id"`
				Email    string `bson:"email"`
				TenantID string `bson:"tenant_id"`
			}
			if errU := s.db.Collection("users").FindOne(ctx, bson.M{"role": "candidate"}).Decode(&uDoc); errU == nil {
				targetUserID = uDoc.ID
				personalEmail = uDoc.Email
				if uDoc.TenantID != "" {
					tenantID = uDoc.TenantID
				}
			}
		}
	}

	if targetUserID == "" {
		targetUserID = "9afbe74b-b9ad-4094-a25e-bd4c63e1794c"
	}
	if personalEmail == "" {
		personalEmail = "bhandarenakul11@gmail.com"
	}

	// 2. Extract Company & Job Information
	company := extractCompanyFromEmail(payload.From, payload.Subject)
	jobTitle := extractJobTitleFromSubject(payload.Subject)

	// 3. Classify Message Category
	category := classifyEmailCategory(payload.Subject, payload.TextBody, payload.HTMLBody)

	// 4. Extract Action URL (e.g. Calendly, CodeSignal, or Status Tracker)
	actionURL := extractActionURL(payload.HTMLBody, payload.TextBody)

	// 5. Build Snippet
	snippet := buildSnippet(payload.TextBody, payload.HTMLBody)

	// 6. Generate Authentic Email Document ID & persist to MongoDB
	emailID := newUUIDString()
	extractedReceiptID := extractEmployerReceiptID(payload.Subject, payload.TextBody, payload.HTMLBody)

	htmlBody := payload.HTMLBody
	if strings.TrimSpace(htmlBody) == "" {
		htmlBody = fmt.Sprintf("<div style=\"font-family: sans-serif; white-space: pre-wrap;\">%s</div>", payload.TextBody)
	}

	if s.db != nil {
		doc := bson.M{
			"_id":             emailID,
			"user_id":         targetUserID,
			"tenant_id":       tenantID,
			"candidate_email": personalEmail,
			"routing_email":   cleanTo,
			"from":            payload.From,
			"subject":         payload.Subject,
			"snippet":         snippet,
			"company":         company,
			"job_title":       jobTitle,
			"category":        category,
			"action_url":      actionURL,
			"receipt_id":      extractedReceiptID,
			"body_html":       htmlBody,
			"received_at":     payload.ReceivedAt,
			"created_at":      now,
		}
		_, _ = s.db.Collection("candidate_emails").InsertOne(ctx, doc)
	}

	// 7. Channel B Correlation: Match incoming confirmation email to active/recent application
	var correlatedAppID string
	var verifiedReceipt string = extractedReceiptID

	isConfirmation := category == "confirmation" || category == "application_confirmed" ||
		strings.Contains(strings.ToLower(payload.Subject), "thank you for applying") ||
		strings.Contains(strings.ToLower(payload.Subject), "application received") ||
		strings.Contains(strings.ToLower(payload.Subject), "received your application")

	if s.db != nil && isConfirmation && company != "" && company != "Employer" {
		searchComp := regexp.QuoteMeta(company)
		filter := bson.M{
			"$and": []bson.M{
				{
					"$or": []bson.M{
						{"user_id": targetUserID},
						{"candidate_email": personalEmail},
					},
				},
				{
					"$or": []bson.M{
						{"company_name": bson.M{"$regex": primitive.Regex{Pattern: searchComp, Options: "i"}}},
						{"company": bson.M{"$regex": primitive.Regex{Pattern: searchComp, Options: "i"}}},
					},
				},
			},
		}

		findOpts := options.FindOne().SetSort(bson.M{"updated_at": -1})
		var appDoc struct {
			ID                string `bson:"_id"`
			Status            string `bson:"status"`
			VerifiedReceiptID string `bson:"verified_receipt_id"`
			CompanyName       string `bson:"company_name"`
			JobTitle          string `bson:"job_title"`
		}

		if errApp := s.db.Collection("applications").FindOne(ctx, filter, findOpts).Decode(&appDoc); errApp == nil && appDoc.ID != "" {
			correlatedAppID = appDoc.ID
			if verifiedReceipt == "" && appDoc.VerifiedReceiptID != "" {
				verifiedReceipt = appDoc.VerifiedReceiptID
			}

			updateFields := bson.M{
				"email_verification_status":      "confirmed",
				"email_message_id":               payload.MessageID,
				"confirmation_email_received_at": payload.ReceivedAt,
				"updated_at":                     now,
			}
			// Elevate status to submitted_confirmed if currently unverified or in_flight
			if appDoc.Status != "submitted_confirmed" {
				updateFields["status"] = "submitted"
				updateFields["canonical_status"] = "SUBMITTED_CONFIRMED"
			}
			if verifiedReceipt != "" {
				updateFields["verified_receipt_id"] = verifiedReceipt
			}

			_, _ = s.db.Collection("applications").UpdateOne(ctx, bson.M{"_id": appDoc.ID}, bson.M{"$set": updateFields})

			// Append timeline audit event
			eventID := fmt.Sprintf("evt-email-%d-%s", now.UnixNano(), appDoc.ID[:8])
			_, _ = s.db.Collection("application_events").InsertOne(ctx, bson.M{
				"_id":            eventID,
				"tenant_id":      tenantID,
				"application_id": appDoc.ID,
				"event_type":     "application.email_verified",
				"from_status":    appDoc.Status,
				"to_status":      "SUBMITTED_CONFIRMED",
				"receipt_id":     verifiedReceipt,
				"actor_type":     "email_verifier",
				"actor_id":       payload.From,
				"occurred_at":    now,
			})

			// Record deduplicated email verification event
			msgID := payload.MessageID
			if msgID == "" {
				msgID = fmt.Sprintf("msg-%d-%s", now.UnixNano(), appDoc.ID[:8])
			}
			_, _ = s.db.Collection("email_verification_events").InsertOne(ctx, bson.M{
				"_id":            msgID,
				"tenant_id":      tenantID,
				"application_id": appDoc.ID,
				"message_id":     msgID,
				"sender":         payload.From,
				"recipient":      personalEmail,
				"subject":        payload.Subject,
				"employer":       company,
				"provider":       detectAtsFromEmail(payload.From, payload.Subject, payload.TextBody),
				"receipt_id":     verifiedReceipt,
				"confidence":     1.0,
				"status":         "verified",
				"received_at":    payload.ReceivedAt,
				"created_at":     now,
			})
		}
	}

	// 8. Auto-Forward to Candidate's Personal Email via SRS Mailer
	forwardStatus := "delivered"
	var deliveryErr string

	mailer := NewDefaultMailer()
	forwardSubject := fmt.Sprintf("[OneRole Forwarded] %s", payload.Subject)
	forwardHTML := fmt.Sprintf(`
<div style="border-bottom: 2px solid #24684f; padding-bottom: 12px; margin-bottom: 16px; font-family: sans-serif;">
  <div style="font-size: 11px; font-weight: 700; color: #15803d; text-transform: uppercase;">OneRole Inbound Carrier Routing</div>
  <div style="font-size: 12px; color: #475569;">Received via: <strong>%s</strong> &rarr; Forwarded to: <strong>%s</strong></div>
  <div style="font-size: 12px; color: #475569;">Original Sender: <strong>%s</strong></div>
</div>
%s
`, cleanTo, personalEmail, payload.From, htmlBody)

	res, err := mailer.SendEmail(ctx, EmailPayload{
		Recipient:   personalEmail,
		Subject:     forwardSubject,
		HTMLBody:    forwardHTML,
		TextBody:    payload.TextBody,
		CompanyName: company,
		JobTitle:    jobTitle,
		ReceiptID:   extractedReceiptID,
	})
	if err != nil || (res != nil && !res.Success) {
		forwardStatus = "pending_transport"
		if err != nil {
			deliveryErr = err.Error()
		} else if res != nil {
			deliveryErr = res.Error
		}
	} else {
		forwardStatus = "forwarded"
	}

	if s.db != nil {
		_, _ = s.db.Collection("candidate_emails").UpdateOne(ctx,
			bson.M{"_id": emailID},
			bson.M{"$set": bson.M{
				"forwarded_to":   personalEmail,
				"forward_status": forwardStatus,
				"forwarded_at":   now,
			}},
		)
	}

	return &InboundEmailResult{
		Success:         true,
		EmailID:         emailID,
		UserID:          targetUserID,
		CandidateEmail:  personalEmail,
		RoutingEmail:    cleanTo,
		Company:         company,
		JobTitle:        jobTitle,
		Category:        category,
		ForwardedTo:     personalEmail,
		ForwardStatus:   forwardStatus,
		DeliveryError:   deliveryErr,
		CorrelatedAppID: correlatedAppID,
		VerifiedReceipt: verifiedReceipt,
		Timestamp:       now,
	}, nil
}

// Helpers for email classification and extraction

func extractCompanyFromEmail(from, subject string) string {
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

	// Try from subject: e.g. "Thank you for applying to Figma"
	lowerSub := strings.ToLower(subject)
	if idx := strings.Index(lowerSub, "applying to "); idx != -1 {
		rest := subject[idx+len("applying to "):]
		tokens := strings.Split(rest, " - ")
		if len(tokens) > 0 && strings.TrimSpace(tokens[0]) != "" {
			return strings.TrimSpace(tokens[0])
		}
	}
	if idx := strings.Index(lowerSub, "interview for "); idx != -1 {
		rest := subject[idx+len("interview for "):]
		if atIdx := strings.Index(strings.ToLower(rest), " at "); atIdx != -1 {
			return strings.TrimSpace(rest[atIdx+4:])
		}
	}

	return "Employer"
}

func extractJobTitleFromSubject(subject string) string {
	lowerSub := strings.ToLower(subject)
	if idx := strings.Index(lowerSub, " - "); idx != -1 {
		return strings.TrimSpace(subject[idx+3:])
	}
	if idx := strings.Index(lowerSub, "for "); idx != -1 {
		rest := subject[idx+4:]
		if atIdx := strings.Index(strings.ToLower(rest), " at "); atIdx != -1 {
			return strings.TrimSpace(rest[:atIdx])
		}
		return strings.TrimSpace(rest)
	}
	return "Software Engineer"
}

func classifyEmailCategory(subject, text, html string) string {
	combined := strings.ToLower(fmt.Sprintf("%s %s %s", subject, text, html))

	if strings.Contains(combined, "interview") || strings.Contains(combined, "schedule a call") || strings.Contains(combined, "phone screen") || strings.Contains(combined, "chat with") {
		return "interview_invite"
	}
	if strings.Contains(combined, "assessment") || strings.Contains(combined, "codesignal") || strings.Contains(combined, "hackerrank") || strings.Contains(combined, "coding challenge") {
		return "assessment"
	}
	if strings.Contains(combined, "offer") || strings.Contains(combined, "compensation") || strings.Contains(combined, "we are pleased to offer") {
		return "offer"
	}
	if strings.Contains(combined, "unfortunately") || strings.Contains(combined, "not moving forward") || strings.Contains(combined, "other candidates") {
		return "rejection"
	}
	return "confirmation"
}

func extractActionURL(html, text string) string {
	content := fmt.Sprintf("%s %s", html, text)
	if m := rxCalendly.FindString(content); m != "" {
		return m
	}
	if m := rxCodeSig.FindString(content); m != "" {
		return m
	}
	if m := rxZoom.FindString(content); m != "" {
		return m
	}
	if m := rxHackerR.FindString(content); m != "" {
		return m
	}
	if m := rxTracker.FindString(content); m != "" {
		return m
	}
	return ""
}

func buildSnippet(text, html string) string {
	clean := text
	if strings.TrimSpace(clean) == "" {
		// strip simple html tags
		r := regexp.MustCompile(`<[^>]*>`)
		clean = r.ReplaceAllString(html, " ")
	}
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 160 {
		return clean[:157] + "..."
	}
	return clean
}
