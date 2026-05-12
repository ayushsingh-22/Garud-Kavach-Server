package services

import "fmt"

// EmailTemplate wraps content in a branded, responsive HTML email layout.
func EmailTemplate(greeting, body, footer string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0;padding:0;background-color:#f4f4f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f7;padding:40px 20px;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.06);">
  <!-- Header -->
  <tr>
    <td style="background-color:#ea580c;padding:24px 32px;text-align:center;">
      <h1 style="margin:0;color:#ffffff;font-size:22px;font-weight:700;letter-spacing:0.5px;">Garud Kavach</h1>
      <p style="margin:4px 0 0;color:#fff3e0;font-size:12px;letter-spacing:1px;">SECURITY SERVICES</p>
    </td>
  </tr>
  <!-- Body -->
  <tr>
    <td style="padding:32px;">
      <h2 style="margin:0 0 16px;color:#1e293b;font-size:18px;font-weight:600;">%s</h2>
      <div style="color:#475569;font-size:15px;line-height:1.7;">%s</div>
    </td>
  </tr>
  <!-- Footer -->
  <tr>
    <td style="padding:20px 32px;background-color:#f8fafc;border-top:1px solid #e2e8f0;">
      <p style="margin:0;color:#94a3b8;font-size:13px;line-height:1.5;">%s</p>
    </td>
  </tr>
  <!-- Bottom -->
  <tr>
    <td style="padding:16px 32px;text-align:center;">
      <p style="margin:0;color:#cbd5e1;font-size:11px;">&copy; Garud Kavach Security Services. All rights reserved.</p>
    </td>
  </tr>
</table>
</td></tr>
</table>
</body>
</html>`, greeting, body, footer)
}

// StatusBadge returns an inline-styled status badge for emails.
func StatusBadge(status string) string {
	color := "#64748b"
	bg := "#f1f5f9"
	switch status {
	case "Pending":
		color = "#d97706"
		bg = "#fef3c7"
	case "In Progress":
		color = "#2563eb"
		bg = "#dbeafe"
	case "Resolved":
		color = "#16a34a"
		bg = "#dcfce7"
	case "Rejected":
		color = "#dc2626"
		bg = "#fee2e2"
	}
	return fmt.Sprintf(
		`<span style="display:inline-block;padding:4px 12px;border-radius:12px;font-size:13px;font-weight:600;color:%s;background-color:%s;">%s</span>`,
		color, bg, status,
	)
}
