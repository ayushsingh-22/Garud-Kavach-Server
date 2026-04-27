package services

import (
	"log"
	"time"
)

// EmailJob represents an email to be sent asynchronously.
type EmailJob struct {
	To       string
	ToName   string
	Subject  string
	HTMLBody string
}

var emailQueue chan EmailJob

// StartEmailWorker initializes the email queue and starts the background worker.
// Call this once at application startup.
func StartEmailWorker() {
	emailQueue = make(chan EmailJob, 100)

	go func() {
		for job := range emailQueue {
			sendWithRetry(job)
		}
	}()

	log.Println("Email worker started.")
}

// EnqueueEmail adds an email job to the async queue. Non-blocking.
// If the queue is full, the email is dropped and logged.
func EnqueueEmail(to, toName, subject, htmlBody string) {
	job := EmailJob{
		To:       to,
		ToName:   toName,
		Subject:  subject,
		HTMLBody: htmlBody,
	}

	select {
	case emailQueue <- job:
		// queued successfully
	default:
		log.Printf("WARNING: Email queue full, dropping email to %s subject=%q", to, subject)
	}
}

func sendWithRetry(job EmailJob) {
	delays := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	for attempt, delay := range delays {
		err := SendEmail(job.To, job.ToName, job.Subject, job.HTMLBody)
		if err == nil {
			return
		}
		log.Printf("WARNING: Email send attempt %d failed for %s: %v", attempt+1, job.To, err)
		time.Sleep(delay)
	}

	log.Printf("ERROR: Failed to send email to %s after 3 attempts. Subject: %q", job.To, job.Subject)
}
