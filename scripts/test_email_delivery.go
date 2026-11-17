//go:build ignore

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oneRole/service"
)

func main() {
	recipientFlag := flag.String("to", "bhandarenakul@gmail.com", "Recipient email address")
	flag.Parse()

	recipient := strings.TrimSpace(*recipientFlag)
	if recipient == "" {
		recipient = "bhandarenakul@gmail.com"
	}

	fmt.Println("======================================================================")
	fmt.Println("oneRole Outbound Email Delivery Diagnostic")
	fmt.Println("======================================================================")

	cfg := service.LoadMailerConfigFromEnv()
	mailer := service.NewMailerWithConfig(cfg)
	status := mailer.GetConfigStatus()

	fmt.Printf("Active Provider : %v\n", status["active_provider"])
	fmt.Printf("SMTP Configured : %v (Host: %v, Port: %v, User: %v)\n",
		status["smtp_configured"], status["smtp_host"], status["smtp_port"], status["smtp_user"])
	fmt.Printf("Resend Enabled  : %v\n", status["resend_enabled"])
	fmt.Printf("SendGrid Enabled: %v\n", status["sendgrid_enabled"])
	fmt.Printf("Ready for Send  : %v\n", status["is_ready"])
	fmt.Println("----------------------------------------------------------------------")

	if !status["is_ready"].(bool) {
		fmt.Println("[WARNING] No outbound email credentials found in environment!")
		fmt.Println("")
		fmt.Println("To receive real emails in your Gmail inbox (bhandarenakul11@gmail.com):")
		fmt.Println("1. Gmail SMTP (Recommended):")
		fmt.Println("   - Go to Google Account -> Security -> 2-Step Verification -> App Passwords")
		fmt.Println("   - Generate a 16-character App Password for 'oneRole'")
		fmt.Println("   - Add the following to your .env file:")
		fmt.Println("     SMTP_HOST=smtp.gmail.com")
		fmt.Println("     SMTP_PORT=587")
		fmt.Println("     SMTP_USER=bhandarenakul@gmail.com")
		fmt.Println("     SMTP_PASS=<your-16-char-app-password>")
		fmt.Println("     SMTP_FROM=oneRole Applications <bhandarenakul@gmail.com>")
		fmt.Println("")
		fmt.Println("2. Or Transactional Email Provider (Resend / SendGrid):")
		fmt.Println("   - RESEND_API_KEY=re_123456789")
		fmt.Println("   - or SENDGRID_API_KEY=SG.123456789")
		fmt.Println("======================================================================")
		os.Exit(1)
	}

	fmt.Printf("Attempting test delivery to: %s...\n", recipient)
	now := time.Now().UTC()
	text, html := service.GenerateConfirmationEmailContent(
		"Staff Distributed Systems & AI Infrastructure Engineer",
		"NVIDIA",
		"app-diag-verify",
		"rcpt-diag-verified-9988",
		now,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := mailer.SendEmail(ctx, service.EmailPayload{
		Recipient:     recipient,
		Subject:       "Verified Test: oneRole Application Confirmation Email",
		TextBody:      text,
		HTMLBody:      html,
		ApplicationID: "app-diag-verify",
		ReceiptID:     "rcpt-diag-verified-9988",
		CompanyName:   "NVIDIA",
		JobTitle:      "Staff Distributed Systems & AI Infrastructure Engineer",
	})

	if err != nil {
		fmt.Printf("\n[FAILURE] Delivery failed: %v\n", err)
		if strings.Contains(err.Error(), "authentication failed") {
			fmt.Println("Hint: Your SMTP username or password/app-password was rejected by the mail server.")
		}
		os.Exit(2)
	}

	fmt.Printf("\n[SUCCESS] Email successfully dispatched!\n")
	fmt.Printf("Provider    : %s\n", result.Provider)
	fmt.Printf("Delivered At: %s\n", result.DeliveredAt.Format(time.RFC3339))
	fmt.Printf("Message ID  : %s\n", result.MessageID)
	fmt.Println("======================================================================")
}
