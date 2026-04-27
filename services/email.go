package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// SendEmail sends an email using the EmailJS REST API.
// Uses raw HTTP to avoid adding an external SDK dependency.
func SendEmail(to, toName, subject, htmlBody string) error {
	serviceID := os.Getenv("EMAILJS_SERVICE_ID")
	templateID := os.Getenv("EMAILJS_TEMPLATE_ID")
	publicKey := os.Getenv("EMAILJS_PUBLIC_KEY")
	privateKey := os.Getenv("EMAILJS_PRIVATE_KEY")

	if serviceID == "" || templateID == "" || publicKey == "" {
		return fmt.Errorf("EmailJS not configured: set EMAILJS_SERVICE_ID, EMAILJS_TEMPLATE_ID, EMAILJS_PUBLIC_KEY")
	}

	templateParams := map[string]string{
		"to_email":  to,
		"to_name":   toName,
		"subject":   subject,
		"message":   htmlBody,
		"from_name": "Garud Kavach",
	}

	body := map[string]interface{}{
		"service_id":      serviceID,
		"template_id":     templateID,
		"user_id":         publicKey,
		"template_params": templateParams,
	}
	if privateKey != "" {
		body["accessToken"] = privateKey
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://api.emailjs.com/api/v1.0/email/send", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("origin", "http://localhost")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("emailjs returned status %d", resp.StatusCode)
	}

	return nil
}
