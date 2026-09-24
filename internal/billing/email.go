package billing

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// ── HTML email ────────────────────────────────────────────────────────────────

func confirmationEmailHTML(userName, userEmail string, plan *Plan, periodEnd time.Time) string {
	fmtXOF := func(n int) string {
		s := fmt.Sprintf("%d", n)
		if len(s) > 3 {
			s = s[:len(s)-3] + " " + s[len(s)-3:]
		}
		return s + " XOF"
	}
	fmtDate := func(t time.Time) string {
		months := []string{"jan", "fév", "mar", "avr", "mai", "jun", "jul", "aoû", "sep", "oct", "nov", "déc"}
		return fmt.Sprintf("%02d %s %d", t.Day(), months[t.Month()-1], t.Year())
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f4f4f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f7;padding:40px 20px;">
  <tr><td align="center">
    <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;">

      <!-- Logo / Header -->
      <tr><td align="center" style="padding-bottom:28px;">
        <span style="font-size:22px;font-weight:700;color:#9b3dff;letter-spacing:-0.5px;">NEXIUM</span>
        <span style="font-size:14px;color:#999;margin-left:6px;">Storage</span>
      </td></tr>

      <!-- Card -->
      <tr><td style="background:#ffffff;border-radius:12px;box-shadow:0 2px 12px rgba(0,0,0,0.07);overflow:hidden;">

        <!-- Card header -->
        <table width="100%%" cellpadding="0" cellspacing="0">
          <tr><td style="background:#9b3dff;padding:28px 32px;">
            <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.75);text-transform:uppercase;letter-spacing:1px;">Confirmation de paiement</p>
            <p style="margin:8px 0 0;font-size:28px;font-weight:700;color:#ffffff;">%s</p>
            <p style="margin:4px 0 0;font-size:14px;color:rgba(255,255,255,0.8);">%s / mois</p>
          </td></tr>
        </table>

        <!-- Card body -->
        <table width="100%%" cellpadding="0" cellspacing="0" style="padding:32px;">
          <tr><td>
            <p style="margin:0 0 8px;font-size:15px;color:#1a1a2e;">Bonjour <strong>%s</strong>,</p>
            <p style="margin:0 0 28px;font-size:14px;color:#555;line-height:1.6;">
              Votre paiement a bien été reçu et votre abonnement <strong>%s</strong> est maintenant actif.
              Retrouvez ci-dessous le récapitulatif de votre transaction.
            </p>

            <!-- Details table -->
            <table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #e8e8ed;border-radius:8px;overflow:hidden;margin-bottom:28px;">
              <tr style="background:#f8f8fb;">
                <td style="padding:10px 16px;font-size:11px;font-weight:600;color:#888;text-transform:uppercase;letter-spacing:0.8px;border-bottom:1px solid #e8e8ed;">Détail</td>
                <td style="padding:10px 16px;font-size:11px;font-weight:600;color:#888;text-transform:uppercase;letter-spacing:0.8px;border-bottom:1px solid #e8e8ed;text-align:right;">Info</td>
              </tr>
              <tr>
                <td style="padding:12px 16px;font-size:13px;color:#444;border-bottom:1px solid #f0f0f5;">Plan souscrit</td>
                <td style="padding:12px 16px;font-size:13px;color:#1a1a2e;font-weight:600;text-align:right;border-bottom:1px solid #f0f0f5;">%s</td>
              </tr>
              <tr style="background:#f8f8fb;">
                <td style="padding:12px 16px;font-size:13px;color:#444;border-bottom:1px solid #f0f0f5;">Montant payé</td>
                <td style="padding:12px 16px;font-size:13px;color:#1a1a2e;font-weight:700;text-align:right;border-bottom:1px solid #f0f0f5;">%s</td>
              </tr>
              <tr>
                <td style="padding:12px 16px;font-size:13px;color:#444;border-bottom:1px solid #f0f0f5;">Stockage inclus</td>
                <td style="padding:12px 16px;font-size:13px;color:#1a1a2e;text-align:right;border-bottom:1px solid #f0f0f5;">%s</td>
              </tr>
              <tr style="background:#f8f8fb;">
                <td style="padding:12px 16px;font-size:13px;color:#444;border-bottom:1px solid #f0f0f5;">Date d'activation</td>
                <td style="padding:12px 16px;font-size:13px;color:#1a1a2e;text-align:right;border-bottom:1px solid #f0f0f5;">%s</td>
              </tr>
              <tr>
                <td style="padding:12px 16px;font-size:13px;color:#444;">Prochain renouvellement</td>
                <td style="padding:12px 16px;font-size:13px;color:#1a1a2e;font-weight:600;text-align:right;">%s</td>
              </tr>
            </table>

            <p style="margin:0 0 6px;font-size:13px;color:#555;line-height:1.6;">
              Le renouvellement <strong>n'est pas automatique</strong>. Pensez à revenir sur votre espace
              <a href="https://console.nexiumai.io/dashboard/billing" style="color:#9b3dff;text-decoration:none;">Billing</a>
              avant cette date pour renouveler votre abonnement.
            </p>
          </td></tr>
        </table>

        <!-- Divider -->
        <table width="100%%" cellpadding="0" cellspacing="0">
          <tr><td style="border-top:1px solid #f0f0f5;padding:20px 32px;">
            <p style="margin:0;font-size:12px;color:#999;line-height:1.5;">
              Vous recevez cet email car un paiement a été effectué sur le compte associé à <strong>%s</strong>.
              Le reçu PDF est joint à cet email.
            </p>
          </td></tr>
        </table>

      </td></tr>

      <!-- Footer -->
      <tr><td align="center" style="padding:24px 0 0;">
        <p style="margin:0;font-size:12px;color:#aaa;">© %d NEXIUM Storage · Abidjan, Côte d'Ivoire</p>
      </td></tr>

    </table>
  </td></tr>
</table>
</body>
</html>`,
		plan.Name,
		fmtXOF(plan.PriceXOF),
		userName,
		plan.Name,
		plan.Name,
		fmtXOF(plan.PriceXOF),
		fmtStorage(plan.StorageBytes),
		fmtDate(time.Now()),
		fmtDate(periodEnd),
		userEmail,
		time.Now().Year(),
	)
}

// ── Reminder emails ───────────────────────────────────────────────────────────

func ExpiryReminderHTML(userName, planName string, daysLeft int, periodEnd time.Time) string {
	fmtDate := func(t time.Time) string {
		months := []string{"jan", "fév", "mar", "avr", "mai", "jun", "jul", "aoû", "sep", "oct", "nov", "déc"}
		return fmt.Sprintf("%02d %s %d", t.Day(), months[t.Month()-1], t.Year())
	}
	urgency := "info"
	if daysLeft <= 2 {
		urgency = "danger"
	} else if daysLeft <= 5 {
		urgency = "warning"
	}
	colors := map[string][2]string{
		"info":    {"#9b3dff", "#f3e8ff"},
		"warning": {"#f59e0b", "#fef3c7"},
		"danger":  {"#ef4444", "#fee2e2"},
	}
	color := colors[urgency]

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f4f4f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f7;padding:40px 20px;">
  <tr><td align="center">
    <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;">
      <tr><td align="center" style="padding-bottom:28px;">
        <span style="font-size:22px;font-weight:700;color:#9b3dff;letter-spacing:-0.5px;">NEXIUM</span>
        <span style="font-size:14px;color:#999;margin-left:6px;">Storage</span>
      </td></tr>
      <tr><td style="background:#ffffff;border-radius:12px;box-shadow:0 2px 12px rgba(0,0,0,0.07);overflow:hidden;">
        <table width="100%%" cellpadding="0" cellspacing="0">
          <tr><td style="background:%s;padding:6px 32px;">
            <p style="margin:0;font-size:13px;color:%s;font-weight:600;">
              Votre abonnement expire dans <strong>%d jour%s</strong>
            </p>
          </td></tr>
        </table>
        <table width="100%%" cellpadding="0" cellspacing="0" style="padding:32px;">
          <tr><td>
            <p style="margin:0 0 8px;font-size:15px;color:#1a1a2e;">Bonjour <strong>%s</strong>,</p>
            <p style="margin:0 0 20px;font-size:14px;color:#555;line-height:1.6;">
              Votre abonnement <strong>%s</strong> arrive à expiration le <strong>%s</strong>.
              Après cette date, votre compte repassera automatiquement sur le plan <strong>Free</strong>
              (10 GB, fichiers ≤ 100 MB).
            </p>
            <table width="100%%" cellpadding="0" cellspacing="0">
              <tr><td align="center" style="padding:8px 0 24px;">
                <a href="https://console.nexiumai.io/dashboard/billing"
                   style="display:inline-block;background:#9b3dff;color:#ffffff;font-size:14px;font-weight:600;
                          text-decoration:none;padding:12px 28px;border-radius:8px;">
                  Renouveler mon abonnement
                </a>
              </td></tr>
            </table>
            <p style="margin:0;font-size:12px;color:#aaa;line-height:1.5;">
              Si vous avez des questions, répondez simplement à cet email.
            </p>
          </td></tr>
        </table>
      </td></tr>
      <tr><td align="center" style="padding:24px 0 0;">
        <p style="margin:0;font-size:12px;color:#aaa;">© %d NEXIUM Storage · Abidjan, Côte d'Ivoire</p>
      </td></tr>
    </table>
  </td></tr>
</table>
</body>
</html>`,
		color[1], color[0],
		daysLeft, pluralS(daysLeft),
		userName, planName,
		fmtDate(periodEnd),
		time.Now().Year(),
	)
}

func pluralS(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// ── Add-on confirmation ───────────────────────────────────────────────────────

func addonConfirmationHTML(userName, userEmail string, addon *StorageAddon) string {
	label := addon.PackageID
	for _, p := range AddonPackages {
		if p.ID == addon.PackageID {
			label = p.Label
			break
		}
	}
	fmtXOF := func(n int) string {
		s := fmt.Sprintf("%d", n)
		if len(s) > 3 {
			s = s[:len(s)-3] + " " + s[len(s)-3:]
		}
		return s + " XOF"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f4f4f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f7;padding:40px 20px;">
  <tr><td align="center">
    <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;">

      <tr><td align="center" style="padding-bottom:28px;">
        <span style="font-size:22px;font-weight:700;color:#9b3dff;letter-spacing:-0.5px;">NEXIUM</span>
        <span style="font-size:14px;color:#999;margin-left:6px;">Storage</span>
      </td></tr>

      <tr><td style="background:#ffffff;border-radius:12px;box-shadow:0 2px 12px rgba(0,0,0,0.07);overflow:hidden;">

        <table width="100%%" cellpadding="0" cellspacing="0">
          <tr><td style="background:#9b3dff;padding:28px 32px;">
            <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.75);text-transform:uppercase;letter-spacing:1px;">Add-on activé</p>
            <p style="margin:8px 0 0;font-size:28px;font-weight:700;color:#ffffff;">%s</p>
            <p style="margin:4px 0 0;font-size:14px;color:rgba(255,255,255,0.8);">%s</p>
          </td></tr>
        </table>

        <table width="100%%" cellpadding="0" cellspacing="0" style="padding:32px;">
          <tr><td>
            <p style="margin:0 0 8px;font-size:15px;color:#1a1a2e;">Bonjour <strong>%s</strong>,</p>
            <p style="margin:0 0 28px;font-size:14px;color:#555;line-height:1.6;">
              Votre espace de stockage a bien été augmenté. Voici le récapitulatif.
            </p>

            <table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #e8e8ed;border-radius:8px;overflow:hidden;margin-bottom:28px;">
              <tr style="background:#f8f8fb;">
                <td style="padding:10px 16px;font-size:11px;font-weight:600;color:#888;text-transform:uppercase;letter-spacing:0.8px;border-bottom:1px solid #e8e8ed;">Détail</td>
                <td style="padding:10px 16px;font-size:11px;font-weight:600;color:#888;text-transform:uppercase;letter-spacing:0.8px;border-bottom:1px solid #e8e8ed;text-align:right;">Info</td>
              </tr>
              <tr>
                <td style="padding:12px 16px;font-size:13px;color:#444;border-bottom:1px solid #f0f0f5;">Stockage ajouté</td>
                <td style="padding:12px 16px;font-size:13px;color:#1a1a2e;font-weight:600;text-align:right;border-bottom:1px solid #f0f0f5;">%s</td>
              </tr>
              <tr style="background:#f8f8fb;">
                <td style="padding:12px 16px;font-size:13px;color:#444;">Montant payé</td>
                <td style="padding:12px 16px;font-size:13px;color:#1a1a2e;font-weight:700;text-align:right;">%s</td>
              </tr>
            </table>

            <p style="margin:0;font-size:12px;color:#999;line-height:1.5;">
              Vous recevez cet email car un paiement a été effectué sur le compte associé à <strong>%s</strong>.
            </p>
          </td></tr>
        </table>

      </td></tr>

      <tr><td align="center" style="padding:24px 0 0;">
        <p style="margin:0;font-size:12px;color:#aaa;">© %d NEXIUM Storage · Abidjan, Côte d'Ivoire</p>
      </td></tr>

    </table>
  </td></tr>
</table>
</body>
</html>`,
		label, fmtXOF(addon.PriceXOF),
		userName,
		fmtStorage(addon.Bytes), fmtXOF(addon.PriceXOF),
		userEmail,
		time.Now().Year(),
	)
}

// ── Grace period reminder ─────────────────────────────────────────────────────

// GracePeriodReminderHTML is exported so the expiry job (separate package) can call it.
func GracePeriodReminderHTML(userName, planName string, daysLeft int, graceEnd time.Time) string {
	fmtDate := func(t time.Time) string {
		months := []string{"jan", "fév", "mar", "avr", "mai", "jun", "jul", "aoû", "sep", "oct", "nov", "déc"}
		return fmt.Sprintf("%02d %s %d", t.Day(), months[t.Month()-1], t.Year())
	}

	urgency := "#e53935" // rouge par défaut
	label := "Urgent"
	if daysLeft >= 15 {
		urgency = "#f57c00"
		label = "Important"
	} else if daysLeft >= 7 {
		urgency = "#fb8c00"
		label = "Attention"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f4f4f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f7;padding:40px 20px;">
  <tr><td align="center">
    <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;">

      <tr><td align="center" style="padding-bottom:28px;">
        <span style="font-size:22px;font-weight:700;color:#9b3dff;letter-spacing:-0.5px;">NEXIUM</span>
        <span style="font-size:14px;color:#999;margin-left:6px;">Storage</span>
      </td></tr>

      <tr><td style="background:#ffffff;border-radius:12px;box-shadow:0 2px 12px rgba(0,0,0,0.07);overflow:hidden;">

        <table width="100%%" cellpadding="0" cellspacing="0">
          <tr><td style="background:%s;padding:28px 32px;">
            <p style="margin:0;font-size:13px;color:rgba(255,255,255,0.8);text-transform:uppercase;letter-spacing:1px;">%s — Suppression de données</p>
            <p style="margin:8px 0 0;font-size:28px;font-weight:700;color:#ffffff;">%d jour%s restant%s</p>
            <p style="margin:4px 0 0;font-size:14px;color:rgba(255,255,255,0.85);">Avant suppression de vos fichiers</p>
          </td></tr>
        </table>

        <table width="100%%" cellpadding="0" cellspacing="0" style="padding:32px;">
          <tr><td>
            <p style="margin:0 0 8px;font-size:15px;color:#1a1a2e;">Bonjour <strong>%s</strong>,</p>
            <p style="margin:0 0 20px;font-size:14px;color:#555;line-height:1.6;">
              Votre abonnement <strong>%s</strong> a expiré. Vos fichiers sont actuellement en lecture seule
              et seront <strong>définitivement supprimés le %s</strong> si vous ne renouvelez pas votre abonnement.
            </p>

            <table width="100%%" cellpadding="0" cellspacing="0" style="background:#fff3e0;border-left:4px solid %s;border-radius:4px;padding:16px;margin-bottom:24px;">
              <tr><td style="font-size:13px;color:#bf360c;line-height:1.6;">
                <strong>Ce qui sera supprimé :</strong> tous vos fichiers, buckets et projets stockés sur NEXIUM Storage.
                Cette action est <strong>irréversible</strong>.
              </td></tr>
            </table>

            <table width="100%%" cellpadding="0" cellspacing="0" style="margin-bottom:28px;">
              <tr>
                <td align="center">
                  <a href="https://console.nexiumai.io/dashboard/billing"
                     style="display:inline-block;background:#06B6D4;color:#ffffff;font-size:14px;font-weight:600;
                            text-decoration:none;padding:14px 32px;border-radius:8px;">
                    Renouveler mon abonnement
                  </a>
                </td>
              </tr>
            </table>

            <p style="margin:0;font-size:12px;color:#999;line-height:1.5;">
              Si vous souhaitez uniquement récupérer vos fichiers avant la suppression, connectez-vous à
              <a href="https://console.nexiumai.io" style="color:#06B6D4;">console.nexiumai.io</a>
              et téléchargez vos données.
            </p>
          </td></tr>
        </table>

        <table width="100%%" cellpadding="0" cellspacing="0" style="padding:20px 32px;border-top:1px solid #f0f0f5;">
          <tr><td align="center" style="font-size:11px;color:#bbb;">
            © %d NEXIUM Storage · <a href="https://console.nexiumai.io/dashboard/billing" style="color:#06B6D4;text-decoration:none;">Gérer mon abonnement</a>
          </td></tr>
        </table>

      </td></tr>
    </table>
  </td></tr>
</table>
</body>
</html>`,
		urgency, label,
		daysLeft, pluralS(daysLeft), pluralS(daysLeft),
		userName, planName, fmtDate(graceEnd),
		urgency,
		time.Now().Year(),
	)
}

// ── PDF receipt ───────────────────────────────────────────────────────────────

type receiptData struct {
	UserName  string
	UserEmail string
	Plan      *Plan
	PeriodEnd time.Time
	PaymentAt time.Time
}

func generateReceiptPDF(d receiptData) []byte {
	fmtDate := func(t time.Time) string {
		months := []string{"Jan", "Fev", "Mar", "Avr", "Mai", "Jun", "Jul", "Aou", "Sep", "Oct", "Nov", "Dec"}
		return fmt.Sprintf("%02d %s %d", t.Day(), months[t.Month()-1], t.Year())
	}

	lines := []pdfLine{
		{text: "NEXIUM STORAGE", size: 18, bold: true, y: 790},
		{text: "console.nexiumai.io", size: 10, y: 770},
		{text: "", y: 750},
		{text: "RECU DE PAIEMENT", size: 13, bold: true, y: 730},
		{text: strings.Repeat("-", 60), size: 10, y: 718},
		{text: fmt.Sprintf("Plan           : %s", d.Plan.Name), size: 10, y: 700},
		{text: fmt.Sprintf("Montant        : %s XOF", fmtXOFPDF(d.Plan.PriceXOF)), size: 10, y: 684},
		{text: fmt.Sprintf("Stockage       : %s", fmtStorage(d.Plan.StorageBytes)), size: 10, y: 668},
		{text: fmt.Sprintf("Date paiement  : %s", fmtDate(d.PaymentAt)), size: 10, y: 652},
		{text: fmt.Sprintf("Valable jusqu  : %s", fmtDate(d.PeriodEnd)), size: 10, y: 636},
		{text: strings.Repeat("-", 60), size: 10, y: 624},
		{text: fmt.Sprintf("Compte         : %s", pdfStr(d.UserName)), size: 10, y: 606},
		{text: fmt.Sprintf("Email          : %s", d.UserEmail), size: 10, y: 590},
		{text: strings.Repeat("-", 60), size: 10, y: 578},
		{text: "Merci de votre confiance.", size: 10, y: 558},
		{text: "Pour toute question : support@nexium.ai", size: 10, y: 542},
	}

	return buildPDF(lines)
}

type pdfLine struct {
	text string
	size int
	bold bool
	y    int
}

func buildPDF(lines []pdfLine) []byte {
	var stream bytes.Buffer
	stream.WriteString("BT\n")
	for _, l := range lines {
		if l.text == "" {
			continue
		}
		size := l.size
		if size == 0 {
			size = 10
		}
		font := "F1"
		if l.bold {
			font = "F2"
		}
		fmt.Fprintf(&stream, "/%s %d Tf\n%d %d Td\n(%s) Tj\n",
			font, size, 50, l.y, pdfStr(l.text))
		// Reset position for next abs move — use Td from origin is not supported directly,
		// so each line uses Td relative to page origin using text matrix trick
	}
	stream.WriteString("ET\n")

	// Re-build with proper absolute positioning using Tm
	var content bytes.Buffer
	content.WriteString("BT\n")
	for _, l := range lines {
		if l.text == "" {
			continue
		}
		size := l.size
		if size == 0 {
			size = 10
		}
		font := "F1"
		if l.bold {
			font = "F2"
		}
		// Tm sets text matrix: 1 0 0 1 x y Tm
		fmt.Fprintf(&content, "/%s %d Tf\n1 0 0 1 50 %d Tm\n(%s) Tj\n",
			font, size, l.y, pdfStr(l.text))
	}
	content.WriteString("ET\n")
	_ = stream // stream was just a draft, use content

	contentBytes := content.Bytes()
	contentLen := len(contentBytes)

	var buf bytes.Buffer

	// PDF header
	buf.WriteString("%PDF-1.4\n")

	// Track object offsets
	off := make([]int, 7)

	off[1] = buf.Len()
	buf.WriteString("1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n")

	off[2] = buf.Len()
	buf.WriteString("2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n")

	off[3] = buf.Len()
	buf.WriteString("3 0 obj\n" +
		"<</Type /Page /Parent 2 0 R /MediaBox [0 0 595 842]\n" +
		"  /Contents 4 0 R\n" +
		"  /Resources <</Font <</F1 5 0 R /F2 6 0 R>>>>>>\n" +
		"endobj\n")

	off[4] = buf.Len()
	fmt.Fprintf(&buf, "4 0 obj\n<</Length %d>>\nstream\n", contentLen)
	buf.Write(contentBytes)
	buf.WriteString("\nendstream\nendobj\n")

	off[5] = buf.Len()
	buf.WriteString("5 0 obj\n" +
		"<</Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding>>\n" +
		"endobj\n")

	off[6] = buf.Len()
	buf.WriteString("6 0 obj\n" +
		"<</Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding>>\n" +
		"endobj\n")

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 7\n")
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off[i])
	}
	fmt.Fprintf(&buf, "trailer\n<</Size 7 /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return buf.Bytes()
}

// pdfStr encode une chaîne pour le flux de contenu PDF (WinAnsiEncoding / Latin-1).
// Convertit les caractères UTF-8 français vers leur équivalent WinAnsi et échappe les caractères spéciaux PDF.
func pdfStr(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\", "(", "\\(", ")", "\\)",
		"é", "\xe9", "è", "\xe8", "ê", "\xea", "ë", "\xeb",
		"à", "\xe0", "â", "\xe2", "ä", "\xe4",
		"î", "\xee", "ï", "\xef",
		"ô", "\xf4", "ö", "\xf6",
		"ù", "\xf9", "û", "\xfb", "ü", "\xfc",
		"ç", "\xe7",
		"É", "\xc9", "È", "\xc8", "Ê", "\xca",
		"À", "\xc0", "Â", "\xc2",
		"Î", "\xce", "Ô", "\xd4", "Ù", "\xd9", "Û", "\xdb",
		"Ç", "\xc7",
		"’", "'", "‘", "'",
		"“", "\"", "”", "\"",
	)
	return r.Replace(s)
}

func fmtStorage(b int64) string {
	if b >= 1_099_511_627_776 {
		return fmt.Sprintf("%d TB", b/1_099_511_627_776)
	}
	if b >= 1_073_741_824 {
		return fmt.Sprintf("%d GB", b/1_073_741_824)
	}
	return fmt.Sprintf("%d MB", b/1_048_576)
}

func fmtXOFPDF(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) > 3 {
		s = s[:len(s)-3] + " " + s[len(s)-3:]
	}
	return s
}
