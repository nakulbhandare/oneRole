package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/quotedprintable"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var (
	rxSearchNums = regexp.MustCompile(`\* SEARCH\s+([\d\s]+)`)
	rxATSDomain  = regexp.MustCompile(`(?i)(greenhouse\.io|ashbyhq\.com|lever\.co|myworkdayjobs\.com|smartrecruiters\.com|recruitee\.com|bamboohr\.com|icims\.com|rippling\.com|personio|teamtailor|workable|jobvite|verification\s+code|security\s+code|application)`)
)

type imapClient struct {
	conn *tls.Conn
	r    *bufio.Reader
	tag  int
}

func dialIMAP(addr string) (*imapClient, error) {
	conn, err := tls.Dial("tcp", addr, &tls.Config{MinVersion: tls.VersionTLS12})
	if err != nil {
		return nil, err
	}
	c := &imapClient{
		conn: conn,
		r:    bufio.NewReaderSize(conn, 64*1024),
	}
	// Read server greeting
	if _, err := c.r.ReadString('\n'); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *imapClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *imapClient) cmd(command string) ([]string, error) {
	c.tag++
	tagStr := fmt.Sprintf("A%04d", c.tag)
	_ = c.conn.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := fmt.Fprintf(c.conn, "%s %s\r\n", tagStr, command); err != nil {
		return nil, err
	}
	var lines []string
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return lines, err
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, tagStr+" ") {
			if !strings.HasPrefix(line, tagStr+" OK") {
				return lines, fmt.Errorf("imap error: %s", line)
			}
			return lines, nil
		}
		lines = append(lines, line)
	}
}

func (c *imapClient) fetchRaw(seqNum string) (string, error) {
	c.tag++
	tagStr := fmt.Sprintf("A%04d", c.tag)
	_ = c.conn.SetDeadline(time.Now().Add(25 * time.Second))
	if _, err := fmt.Fprintf(c.conn, "%s FETCH %s BODY.PEEK[]\r\n", tagStr, seqNum); err != nil {
		return "", err
	}
	var buf strings.Builder
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return buf.String(), err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(trimmed, tagStr+" ") {
			if !strings.HasPrefix(trimmed, tagStr+" OK") {
				return buf.String(), fmt.Errorf("imap fetch error: %s", trimmed)
			}
			return buf.String(), nil
		}
		buf.WriteString(line)
	}
}

func parseEmail(raw string) (from, to, subject, body string) {
	parts := strings.SplitN(raw, "\r\n\r\n", 2)
	if len(parts) < 2 {
		parts = strings.SplitN(raw, "\n\n", 2)
	}
	headers := parts[0]
	rawBody := ""
	if len(parts) > 1 {
		rawBody = parts[1]
	}

	// Unfold headers
	unfolded := regexp.MustCompile(`\r?\n[ \t]+`).ReplaceAllString(headers, " ")
	var cte string
	for _, line := range strings.Split(unfolded, "\n") {
		line = strings.TrimRight(line, "\r")
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "from:"):
			from = strings.TrimSpace(line[5:])
		case strings.HasPrefix(lower, "to:"):
			to = strings.TrimSpace(line[3:])
		case strings.HasPrefix(lower, "subject:"):
			subject = strings.TrimSpace(line[8:])
		case strings.HasPrefix(lower, "content-transfer-encoding:"):
			cte = strings.ToLower(strings.TrimSpace(line[26:]))
		}
	}

	body = rawBody
	if strings.Contains(cte, "quoted-printable") {
		if decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(rawBody))); err == nil {
			body = string(decoded)
		}
	} else if strings.Contains(cte, "base64") {
		clean := strings.ReplaceAll(strings.ReplaceAll(rawBody, "\r", ""), "\n", "")
		if decoded, err := base64.StdEncoding.DecodeString(clean); err == nil {
			body = string(decoded)
		}
	}
	// Extract email address from "Name <email@domain>"
	if lt := strings.LastIndex(from, "<"); lt != -1 {
		if gt := strings.Index(from[lt:], ">"); gt != -1 {
			from = from[lt+1 : lt+gt]
		}
	}
	if lt := strings.LastIndex(to, "<"); lt != -1 {
		if gt := strings.Index(to[lt:], ">"); gt != -1 {
			to = to[lt+1 : lt+gt]
		}
	}
	return strings.TrimSpace(from), strings.TrimSpace(to), strings.TrimSpace(subject), body
}

func postWebhook(httpClient *http.Client, apiURL, recipient, sender, subject, body string) {
	payload := map[string]string{
		"recipient": recipient,
		"sender":    sender,
		"subject":   subject,
		"body":      body,
		"text":      body,
	}
	b, _ := json.Marshal(payload)

	endpoints := []string{
		strings.TrimRight(apiURL, "/") + "/api/v1/auth/email-otp/webhook",
		strings.TrimRight(apiURL, "/") + "/api/v1/inbound-email/webhook",
	}
	for _, ep := range endpoints {
		req, err := http.NewRequest(http.MethodPost, ep, bytes.NewReader(b))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[EmailIMAP] webhook POST %s failed: %v", ep, err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		log.Printf("[EmailIMAP] Delivered email (%q from %s) -> %s [%d]", subject, sender, ep, resp.StatusCode)
	}
}

func main() {
	imapAddr := os.Getenv("IMAP_ADDR")
	if imapAddr == "" {
		imapAddr = "imap.gmail.com:993"
	}
	imapUser := os.Getenv("IMAP_USER")
	if imapUser == "" {
		imapUser = "bhandarenakul11@gmail.com"
	}
	imapPass := strings.TrimSpace(os.Getenv("IMAP_PASS"))
	apiURL := os.Getenv("ONEROLE_API_URL")
	if apiURL == "" {
		apiURL = "http://onerole-api:8088"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "3002"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Healthcheck server
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"onerole-email-imap-watcher"}`))
	})
	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		log.Printf("[EmailIMAP] Healthcheck listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[EmailIMAP] HTTP server error: %v", err)
		}
	}()

	httpClient := &http.Client{Timeout: 8 * time.Second}
	seenSeqs := make(map[string]bool)

	for {
		if ctx.Err() != nil {
			break
		}
		if imapPass == "" {
			log.Printf("[EmailIMAP] IMAP_PASS not configured for %s — waiting for secret update...", imapUser)
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
				imapPass = strings.TrimSpace(os.Getenv("IMAP_PASS"))
				continue
			}
		}

		client, err := dialIMAP(imapAddr)
		if err != nil {
			log.Printf("[EmailIMAP] Dial %s error: %v (retrying in 5s)", imapAddr, err)
			time.Sleep(5 * time.Second)
			continue
		}

		if _, err := client.cmd(fmt.Sprintf(`LOGIN "%s" "%s"`, imapUser, imapPass)); err != nil {
			log.Printf("[EmailIMAP] LOGIN failed for %s: %v (retrying in 15s)", imapUser, err)
			client.Close()
			time.Sleep(15 * time.Second)
			continue
		}
		if _, err := client.cmd("SELECT INBOX"); err != nil {
			log.Printf("[EmailIMAP] SELECT INBOX failed: %v", err)
			client.Close()
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("[EmailIMAP] Connected & watching INBOX for %s -> %s", imapUser, apiURL)

		for ctx.Err() == nil {
			lines, err := client.cmd("SEARCH UNSEEN")
			if err != nil {
				log.Printf("[EmailIMAP] SEARCH error: %v — reconnecting", err)
				break
			}
			for _, l := range lines {
				m := rxSearchNums.FindStringSubmatch(l)
				if len(m) < 2 {
					continue
				}
				for _, num := range strings.Fields(m[1]) {
					if seenSeqs[num] {
						continue
					}
					raw, fErr := client.fetchRaw(num)
					if fErr != nil {
						continue
					}
					seenSeqs[num] = true
					from, to, subject, body := parseEmail(raw)
					if to == "" {
						to = imapUser
					}
					if rxATSDomain.MatchString(from + " " + subject + " " + body) {
						postWebhook(httpClient, apiURL, to, from, subject, body)
					}
				}
			}
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
		client.Close()
	}
	_ = srv.Shutdown(context.Background())
}
