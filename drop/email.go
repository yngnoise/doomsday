package drop

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// Mailer sends transactional emails via Yandex SMTP (or any SMTP provider).
// Required env vars:
//   SMTP_HOST  — e.g. smtp.yandex.ru
//   SMTP_PORT  — 465 (SSL) or 587 (STARTTLS). Yandex uses 465.
//   SMTP_USER  — your full Yandex address, e.g. you@yandex.ru
//   SMTP_PASS  — app password from Yandex security settings
//   EMAIL_FROM — display name + address, e.g. "DOOMSDAY™ <you@yandex.ru>"
//   SITE_URL   — e.g. http://localhost:3000

type Mailer struct {
	host    string
	port    string
	user    string
	pass    string
	from    string
	siteURL string
	logger  *slog.Logger
}

func NewMailer(logger *slog.Logger) *Mailer {
	siteURL := os.Getenv("SITE_URL")
	if siteURL == "" {
		siteURL = "http://localhost:3000"
	}
	return &Mailer{
		host:    os.Getenv("SMTP_HOST"),
		port:    os.Getenv("SMTP_PORT"),
		user:    os.Getenv("SMTP_USER"),
		pass:    os.Getenv("SMTP_PASS"),
		from:    os.Getenv("EMAIL_FROM"),
		siteURL: siteURL,
		logger:  logger,
	}
}

func (m *Mailer) Enabled() bool {
	return m.host != "" && m.user != "" && m.pass != ""
}

// ─────────────────────────────────────────────────────────────────────────────
// TRANSPORT — SSL on port 465 (Yandex default)
// ─────────────────────────────────────────────────────────────────────────────

func (m *Mailer) send(ctx context.Context, to, subject, html string) error {
	if !m.Enabled() {
		m.logger.InfoContext(ctx, "email skipped (SMTP not configured)", slog.String("to", to))
		return nil
	}

	port := m.port
	if port == "" {
		port = "465"
	}

	addr := net.JoinHostPort(m.host, port)

	// Build raw MIME message
	msg := buildMIME(m.from, to, subject, html)

	auth := smtp.PlainAuth("", m.user, m.pass, m.host)

	if port == "465" {
		// Implicit TLS (Yandex SMTP)
		tlsConf := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         m.host,
		}
		conn, err := tls.Dial("tcp", addr, tlsConf)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, m.host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()

		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
		if err := client.Mail(m.user); err != nil {
			return fmt.Errorf("smtp MAIL: %w", err)
		}
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("smtp RCPT: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp DATA: %w", err)
		}
		if _, err := w.Write([]byte(msg)); err != nil {
			return fmt.Errorf("smtp write: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("smtp close: %w", err)
		}
		return client.Quit()
	}

	// STARTTLS fallback (port 587)
	return smtp.SendMail(addr, auth, m.user, []string{to}, []byte(msg))
}

func buildMIME(from, to, subject, html string) string {
	msgID := fmt.Sprintf("<%d.doomsday@%s>", time.Now().UnixNano(), "yandex.ru")
	var sb strings.Builder
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	sb.WriteString(fmt.Sprintf("Message-ID: %s\r\n", msgID))
	sb.WriteString(fmt.Sprintf("From: %s\r\n", from))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", to))
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	sb.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z)))
	sb.WriteString("X-Mailer: DOOMSDAY-Mailer/1.0\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(html)
	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// BASE LAYOUT
// All styles inline — Gmail strips <style> blocks.
// bgcolor on <table> for Outlook compatibility alongside inline style.
// ─────────────────────────────────────────────────────────────────────────────

func emailBase(subject, content string) string {
	return `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <meta name="color-scheme" content="dark" />
  <meta name="supported-color-schemes" content="dark" />
  <title>DOOMSDAY</title>
</head>
<body style="margin:0;padding:0;background-color:#000000 !important;-webkit-text-size-adjust:100%;" bgcolor="#000000">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" bgcolor="#000000"
  style="background-color:#000000 !important;min-height:100vh;">
  <tr>
    <td align="center" style="padding:32px 16px;">
      <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="560"
        style="max-width:560px;width:100%;background-color:#000000;border:1px solid #3f3f46;">
        <tr>
          <td>
            <!-- Top accent bar -->
            <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%">
              <tr><td height="2" style="background-color:#ffffff;font-size:0;line-height:0;">&nbsp;</td></tr>
            </table>
            <!-- Header -->
            <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%">
              <tr>
                <td style="padding:24px 36px 20px;border-bottom:1px solid #3f3f46;">
                  <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%">
                    <tr>
                      <td>
                        <span style="font-family:Impact,'Arial Black',Arial,sans-serif;font-size:20px;font-weight:900;color:#ffffff;letter-spacing:0.1em;text-transform:uppercase;">DOOMSDAY</span>
                      </td>
                      <td align="right">
                        <span style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.2em;text-transform:uppercase;">SS/25</span>
                      </td>
                    </tr>
                  </table>
                </td>
              </tr>
            </table>
            <!-- Subject line -->
            <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%">
              <tr>
                <td style="padding:16px 36px;border-bottom:1px solid #27272a;background-color:#0a0a0a;">
                  <span style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#a1a1aa;letter-spacing:0.2em;text-transform:uppercase;">` + subject + `</span>
                </td>
              </tr>
            </table>
            <!-- Content -->
            <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%">
              <tr>
                <td style="padding:36px 36px 0;">` +
		content +
		`</td>
              </tr>
            </table>
            <!-- Footer -->
            <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%">
              <tr>
                <td style="padding:24px 36px 28px;border-top:1px solid #27272a;margin-top:36px;">
                  <p style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.15em;text-transform:uppercase;margin:0;">
                    &copy; DOOMSDAY 2025 &nbsp;&middot;&nbsp; No restocks &nbsp;&middot;&nbsp; Ever.
                  </p>
                </td>
              </tr>
            </table>
          </td>
        </tr>
      </table>
    </td>
  </tr>
</table>
</body>
</html>`
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func emailRow(label, value, valueColor string) string {
	if valueColor == "" {
		valueColor = "#d4d4d8"
	}
	return fmt.Sprintf(`
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin-bottom:20px;">
  <tr><td>
    <span style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.2em;text-transform:uppercase;display:block;margin-bottom:6px;">%s</span>
    <span style="font-family:'Courier New',Courier,monospace;font-size:14px;color:%s;display:block;">%s</span>
  </td></tr>
</table>`, label, valueColor, value)
}

func emailDivider() string {
	return `<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" style="margin:28px 0;">
  <tr><td height="1" style="background-color:#3f3f46;font-size:0;line-height:0;">&nbsp;</td></tr>
</table>`
}

func emailCTA(label, url string) string {
	return fmt.Sprintf(`
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin:28px 0;">
  <tr><td>
    <a href="%s" style="display:block;background-color:#ffffff;color:#000000;font-family:'Courier New',Courier,monospace;font-size:12px;font-weight:700;letter-spacing:0.3em;text-transform:uppercase;text-align:center;padding:18px 24px;text-decoration:none;">
      %s
    </a>
  </td></tr>
</table>`, url, label)
}

func emailAlert(text, color string) string {
	return fmt.Sprintf(`
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin-bottom:24px;">
  <tr><td style="border-left:3px solid %s;padding:12px 16px;background-color:#0a0a0a;">
    <span style="font-family:'Courier New',Courier,monospace;font-size:12px;color:%s;letter-spacing:0.05em;line-height:1.6;">%s</span>
  </td></tr>
</table>`, color, color, text)
}

// ─────────────────────────────────────────────────────────────────────────────
// OTP
// ─────────────────────────────────────────────────────────────────────────────

func (m *Mailer) SendOTP(ctx context.Context, to, code string) {
	if !m.Enabled() {
		m.logger.WarnContext(ctx, "SMTP not configured — OTP code (dev mode)",
			slog.String("to", to), slog.String("code", code))
		return
	}
	go func() {
		digits := ""
		for i, ch := range code {
			if i > 0 {
				digits += `<span style="display:inline-block;width:12px;"></span>`
			}
			digits += fmt.Sprintf(
				`<span style="font-family:Impact,'Arial Black',Arial,sans-serif;font-size:64px;font-weight:900;color:#ffffff;line-height:1;">%s</span>`,
				string(ch),
			)
		}
		requestedAt := time.Now().UTC().Format("02 Jan 2006 · 15:04 UTC")
		expiresAt := time.Now().UTC().Add(10 * time.Minute).Format("15:04 UTC")

		content := fmt.Sprintf(`
<p style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.25em;text-transform:uppercase;margin:0 0 10px;">Authentication Request</p>
<p style="font-family:Impact,'Arial Black',Arial,sans-serif;font-size:56px;font-weight:900;color:#ffffff;line-height:0.95;text-transform:uppercase;margin:0 0 36px;">ACCESS<br/>CODE</p>
<p style="font-family:'Courier New',Courier,monospace;font-size:13px;color:#a1a1aa;line-height:1.8;margin:0 0 32px;">
  A one-time code has been issued for your DOOMSDAY account.<br/>
  Valid for <span style="color:#ffffff;font-weight:700;">10&nbsp;minutes</span>.
</p>
%s
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin-bottom:12px;">
  <tr><td style="border:1px solid #3f3f46;background-color:#09090b;padding:40px 24px;text-align:center;">
    %s
    <br/>
    <span style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.2em;text-transform:uppercase;display:inline-block;margin-top:20px;">Expires at %s</span>
  </td></tr>
</table>
<p style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.15em;text-align:center;margin:0 0 32px;">Enter this code on the DOOMSDAY site to authenticate</p>
%s
<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin:0 0 8px;border-top:1px solid #27272a;padding-top:24px;">
  <tr>
    <td width="50%%" style="padding-top:20px;padding-bottom:16px;">
      <span style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.15em;text-transform:uppercase;display:block;margin-bottom:6px;">Requested</span>
      <span style="font-family:'Courier New',Courier,monospace;font-size:12px;color:#a1a1aa;">%s</span>
    </td>
    <td width="50%%" align="right" style="padding-top:20px;padding-bottom:16px;">
      <span style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.15em;text-transform:uppercase;display:block;margin-bottom:6px;">Sent to</span>
      <span style="font-family:'Courier New',Courier,monospace;font-size:12px;color:#a1a1aa;">%s</span>
    </td>
  </tr>
</table>
<p style="font-family:'Courier New',Courier,monospace;font-size:12px;color:#52525b;line-height:1.6;margin:0 0 36px;">
  If you did not request this code, you can safely ignore this email.
</p>`,
			emailDivider(), digits, expiresAt,
			emailAlert("Do not share this code. DOOMSDAY staff will never ask for it.", "#52525b"),
			requestedAt, to,
		)

		if err := m.send(context.Background(), to,
			"DOOMSDAY: ваш код подтверждения",
			emailBase("Access Code", content)); err != nil {
			m.logger.Error("OTP email failed", slog.String("to", to), slog.Any("err", err))
		}
	}()
}

// ─────────────────────────────────────────────────────────────────────────────
// RESERVATION CONFIRMATION
// ─────────────────────────────────────────────────────────────────────────────

func (m *Mailer) SendReservationConfirmation(ctx context.Context, to, name, itemName, reservationID string, expiresAt time.Time) {
	go func() {
		minutesLeft := int(time.Until(expiresAt).Minutes()) + 1
		checkoutURL := fmt.Sprintf("%s/checkout/%s?expires=%s",
			m.siteURL, reservationID, expiresAt.UTC().Format(time.RFC3339))

		content := fmt.Sprintf(`
<p style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.25em;text-transform:uppercase;margin:0 0 8px;">Status Update</p>
<p style="font-family:Impact,'Arial Black',Arial,sans-serif;font-size:48px;font-weight:900;color:#ffffff;line-height:0.95;margin:0 0 32px;">ITEM<br/>SECURED</p>
%s%s%s%s%s%s%s`,
			emailRow("Item", itemName, "#ffffff"),
			emailRow("Reservation ID", fmt.Sprintf(`<span style="font-family:'Courier New',Courier,monospace;font-size:12px;color:#a1a1aa;">%s</span>`, reservationID), ""),
			emailRow("Status", `<span style="color:#22c55e;font-weight:700;">&#10003; Reserved</span>`, ""),
			emailRow("Expires", expiresAt.UTC().Format("02 Jan 2006 · 15:04 UTC"), "#ef4444"),
			emailAlert(fmt.Sprintf("You have %d minutes to complete checkout. After that your reservation will be released.", minutesLeft), "#ef4444"),
			emailCTA("Complete Checkout", checkoutURL),
			emailDivider(),
		)

		if err := m.send(context.Background(), to,
			"Item secured — complete checkout now",
			emailBase("Reservation Confirmed", content)); err != nil {
			m.logger.Error("reservation email failed", slog.String("to", to), slog.Any("err", err))
		}
	}()
}

// ─────────────────────────────────────────────────────────────────────────────
// ORDER CONFIRMATION
// ─────────────────────────────────────────────────────────────────────────────

func (m *Mailer) SendOrderConfirmation(ctx context.Context, to, name, itemName, orderID string, priceCents int) {
	go func() {
		price := fmt.Sprintf("$%d", priceCents/100)
		dispatch := time.Now().AddDate(0, 0, 2).Format("02 Jan 2006")

		content := fmt.Sprintf(`
<p style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.25em;text-transform:uppercase;margin:0 0 8px;">Order Confirmed</p>
<p style="font-family:Impact,'Arial Black',Arial,sans-serif;font-size:48px;font-weight:900;color:#ffffff;line-height:0.95;margin:0 0 32px;">ORDER<br/>LOCKED IN</p>
%s%s%s%s%s%s%s
<p style="font-family:'Courier New',Courier,monospace;font-size:12px;color:#71717a;line-height:1.7;margin:0 0 36px;">
  This is a one-of-a-kind item from a limited production run.<br/>No restocks. No returns. No exceptions.
</p>`,
			emailRow("Order ID", orderID, "#ffffff"),
			emailRow("Item", itemName, "#ffffff"),
			emailDivider(),
			emailRow("Subtotal", price, "#d4d4d8"),
			emailRow("Shipping", "Free — tracked worldwide", "#d4d4d8"),
			emailRow("Estimated Dispatch", dispatch, "#d4d4d8"),
			emailDivider(),
		)

		if err := m.send(context.Background(), to,
			fmt.Sprintf("Order %s confirmed — DOOMSDAY", orderID),
			emailBase("Order Confirmation", content)); err != nil {
			m.logger.Error("order email failed", slog.String("to", to), slog.Any("err", err))
		}
	}()
}

// ─────────────────────────────────────────────────────────────────────────────
// WAITLIST PROMOTION
// ─────────────────────────────────────────────────────────────────────────────

func (m *Mailer) SendWaitlistPromotion(ctx context.Context, to, dropID, dropName string) {
	go func() {
		dropURL := fmt.Sprintf("%s/drops/%s", m.siteURL, dropID)

		content := fmt.Sprintf(`
<p style="font-family:'Courier New',Courier,monospace;font-size:11px;color:#71717a;letter-spacing:0.25em;text-transform:uppercase;margin:0 0 8px;">Waitlist</p>
<p style="font-family:Impact,'Arial Black',Arial,sans-serif;font-size:48px;font-weight:900;color:#ffffff;line-height:0.95;margin:0 0 32px;">YOUR<br/>SLOT IS OPEN</p>
<p style="font-family:'Courier New',Courier,monospace;font-size:13px;color:#a1a1aa;line-height:1.7;margin:0 0 24px;">
  A unit of <span style="color:#ffffff;font-weight:700;">%s</span> has become available.<br/>You are next in the queue.
</p>
%s%s%s`,
			dropName,
			emailAlert("This window is time-limited. The unit will be released to the next person if not claimed.", "#eab308"),
			emailCTA("Claim Your Unit", dropURL),
			emailDivider(),
		)

		if err := m.send(context.Background(), to,
			"Your waitlist slot is open — "+dropName,
			emailBase("Waitlist Notification", content)); err != nil {
			m.logger.Error("waitlist email failed", slog.String("to", to), slog.Any("err", err))
		}
	}()
}
