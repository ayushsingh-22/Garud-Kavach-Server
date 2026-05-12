package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// StartGuardLicenseChecker runs a daily check at 9 AM for guards
// whose licenses expire within 7 days and notifies HR users via email.
func StartGuardLicenseChecker(database *sql.DB) {
	go func() {
		for {
			now := time.Now()
			next9AM := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
			if now.After(next9AM) {
				next9AM = next9AM.Add(24 * time.Hour)
			}
			waitDuration := next9AM.Sub(now)
			hours := int(waitDuration.Hours())
			minutes := int(waitDuration.Minutes()) % 60
			log.Printf("Guard license checker: next run at %s (in %dh %dm)", next9AM.Format("02 Jan 2006, 03:04 PM"), hours, minutes)

			timer := time.NewTimer(waitDuration)
			<-timer.C

			checkExpiringLicenses(database)
		}
	}()

	log.Println("Guard license expiry checker scheduled.")
}

// CheckExpiringLicenses checks for guards with licenses expiring within 7 days and notifies HR.
func CheckExpiringLicenses(database *sql.DB) {
	checkExpiringLicenses(database)
}

func checkExpiringLicenses(database *sql.DB) {
	sevenDaysFromNow := time.Now().Add(7 * 24 * time.Hour)

	rows, err := database.Query(
		`SELECT g.name, g.license_no, g.license_expiry
		 FROM guards g
		 WHERE g.deleted_at IS NULL
		   AND g.license_expiry IS NOT NULL
		   AND g.license_expiry <= $1
		   AND g.license_expiry > NOW()`,
		sevenDaysFromNow,
	)
	if err != nil {
		log.Printf("ERROR: Guard license check query failed: %v", err)
		return
	}
	defer rows.Close()

	type expiringGuard struct {
		Name          string
		LicenseNo     string
		LicenseExpiry time.Time
	}

	var expiring []expiringGuard
	for rows.Next() {
		var g expiringGuard
		var licNo sql.NullString
		if err := rows.Scan(&g.Name, &licNo, &g.LicenseExpiry); err != nil {
			log.Printf("ERROR: Failed to scan expiring guard: %v", err)
			continue
		}
		if licNo.Valid {
			g.LicenseNo = licNo.String
		}
		expiring = append(expiring, g)
	}

	if len(expiring) == 0 {
		log.Println("Guard license check: no expiring licenses found.")
		return
	}

	// Build HTML email body
	tableRows := ""
	for _, g := range expiring {
		licInfo := ""
		if g.LicenseNo != "" {
			licInfo = g.LicenseNo
		}
		tableRows += fmt.Sprintf(
			`<tr><td style="padding:10px 12px;border-bottom:1px solid #e2e8f0;font-weight:600;">%s</td>`+
				`<td style="padding:10px 12px;border-bottom:1px solid #e2e8f0;">%s</td>`+
				`<td style="padding:10px 12px;border-bottom:1px solid #e2e8f0;color:#dc2626;font-weight:600;">%s</td></tr>`,
			g.Name, licInfo, g.LicenseExpiry.Format("02 Jan 2006"),
		)
	}
	emailContent := `<p>The following guards have licenses expiring within <strong>7 days</strong>. Please take action immediately.</p>` +
		`<table style="margin:16px 0;border-collapse:collapse;width:100%;font-size:14px;">` +
		`<tr style="background-color:#f8fafc;"><th style="padding:10px 12px;text-align:left;color:#64748b;font-size:12px;text-transform:uppercase;">Guard Name</th>` +
		`<th style="padding:10px 12px;text-align:left;color:#64748b;font-size:12px;text-transform:uppercase;">License No.</th>` +
		`<th style="padding:10px 12px;text-align:left;color:#64748b;font-size:12px;text-transform:uppercase;">Expiry Date</th></tr>` +
		tableRows + `</table>`
	body := EmailTemplate("License Expiry Alert", emailContent, fmt.Sprintf("%d guard(s) require immediate attention.", len(expiring)))

	// Send to all HR users
	hrRows, err := database.Query("SELECT email, COALESCE(name, email) FROM users WHERE role = 'hr' AND deleted_at IS NULL")
	if err != nil {
		log.Printf("ERROR: Failed to query HR users for license check: %v", err)
		return
	}
	defer hrRows.Close()

	for hrRows.Next() {
		var email, name string
		if err := hrRows.Scan(&email, &name); err != nil {
			continue
		}
		EnqueueEmail(email, name, "Guard License Expiry Alert", body)
	}

	log.Printf("Guard license check: notified HR about %d expiring licenses.", len(expiring))
}
