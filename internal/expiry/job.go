package expiry

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"nexium.ai/api/internal/mailer"
	"nexium.ai/api/internal/storage"
)

type inactiveUser struct {
	ID    uuid.UUID
	Email string
	Name  string
}

type Job struct {
	db      *pgxpool.Pool
	r2      *storage.R2Client
	mail    *mailer.Mailer
	appName string
}

func New(db *pgxpool.Pool, r2 *storage.R2Client, mail *mailer.Mailer, appName string) *Job {
	return &Job{db: db, r2: r2, mail: mail, appName: appName}
}

// Start lance le job d'expiration toutes les 24h. À appeler dans une goroutine.
func (j *Job) Start(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Première exécution au démarrage
	j.run(ctx)

	for {
		select {
		case <-ticker.C:
			j.run(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (j *Job) run(ctx context.Context) {
	log.Println("[expiry] running daily check")
	j.expireSubscriptions(ctx)
	j.remindExpiringSubscriptions(ctx)
	j.warnInactiveUsers(ctx)
	j.deleteExpiredFiles(ctx)
}

// expireSubscriptions expire les abonnements dont current_period_end est dépassé
// et remet le quota de l'utilisateur au niveau Free (les add-ons sont suspendus, pas supprimés).
func (j *Job) expireSubscriptions(ctx context.Context) {
	type expiredSub struct {
		userID    uuid.UUID
		freeBytes int64
	}

	rows, err := j.db.Query(ctx, `
		SELECT s.user_id, fp.storage_bytes
		FROM subscriptions s
		CROSS JOIN (SELECT storage_bytes FROM plans WHERE slug = 'free' LIMIT 1) fp
		WHERE s.status = 'active' AND s.current_period_end < now()
	`)
	if err != nil {
		log.Printf("[expiry] expire subscriptions query error: %v", err)
		return
	}
	defer rows.Close()

	var subs []expiredSub
	for rows.Next() {
		var e expiredSub
		if err := rows.Scan(&e.userID, &e.freeBytes); err == nil {
			subs = append(subs, e)
		}
	}
	rows.Close()

	for _, e := range subs {
		if _, err := j.db.Exec(ctx,
			`UPDATE subscriptions SET status = 'expired', updated_at = now() WHERE user_id = $1 AND status = 'active'`,
			e.userID,
		); err != nil {
			log.Printf("[expiry] expire subscription error user=%s: %v", e.userID, err)
			continue
		}
		if _, err := j.db.Exec(ctx,
			`UPDATE users SET storage_quota_bytes = $2, updated_at = now() WHERE id = $1`,
			e.userID, e.freeBytes,
		); err != nil {
			log.Printf("[expiry] reset quota error user=%s: %v", e.userID, err)
			continue
		}
		log.Printf("[expiry] expired subscription user=%s quota→%d bytes (Free)", e.userID, e.freeBytes)
	}
}

// remindExpiringSubscriptions envoie des rappels email à 10, 5 et 2 jours avant l'expiration.
func (j *Job) remindExpiringSubscriptions(ctx context.Context) {
	type reminder struct {
		days   int
		column string
		window [2]int // [minDays, maxDays] from now
	}
	reminders := []reminder{
		{days: 10, column: "reminded_10d_at", window: [2]int{9, 11}},
		{days: 5, column: "reminded_5d_at", window: [2]int{4, 6}},
		{days: 2, column: "reminded_2d_at", window: [2]int{1, 3}},
	}

	for _, rem := range reminders {
		query := fmt.Sprintf(`
			SELECT s.user_id, u.name, u.email, p.name, s.current_period_end
			FROM subscriptions s
			JOIN users u ON u.id = s.user_id
			JOIN plans p ON p.id = s.plan_id
			WHERE s.status = 'active'
			  AND s.current_period_end IS NOT NULL
			  AND s.current_period_end BETWEEN now() + interval '%d days' AND now() + interval '%d days'
			  AND s.%s IS NULL
		`, rem.window[0], rem.window[1], rem.column)

		rows, err := j.db.Query(ctx, query)
		if err != nil {
			log.Printf("[expiry] remind-%dd query error: %v", rem.days, err)
			continue
		}

		type sub struct {
			userID    uuid.UUID
			name      string
			email     string
			planName  string
			periodEnd time.Time
		}
		var subs []sub
		for rows.Next() {
			var s sub
			if err := rows.Scan(&s.userID, &s.name, &s.email, &s.planName, &s.periodEnd); err == nil {
				subs = append(subs, s)
			}
		}
		rows.Close()

		for _, s := range subs {
			months := []string{"jan", "fev", "mar", "avr", "mai", "jun", "jul", "aou", "sep", "oct", "nov", "dec"}
			dateStr := fmt.Sprintf("%02d %s %d", s.periodEnd.Day(), months[s.periodEnd.Month()-1], s.periodEnd.Year())

			subject := fmt.Sprintf("[NEXIUM Storage] Votre abonnement expire dans %d jour%s (%s)",
				rem.days, pluralS(rem.days), dateStr)

			body := buildReminderEmailHTML(s.name, s.planName, rem.days, s.periodEnd)

			if err := j.mail.Send(ctx, s.email, s.name, subject, body); err != nil {
				log.Printf("[expiry] remind-%dd email error user=%s: %v", rem.days, s.userID, err)
				continue
			}

			j.db.Exec(ctx,
				fmt.Sprintf(`UPDATE subscriptions SET %s = now() WHERE user_id = $1`, rem.column),
				s.userID,
			)
			log.Printf("[expiry] sent %dd reminder user=%s", rem.days, s.userID)
		}
	}
}

func pluralS(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

func buildReminderEmailHTML(name, planName string, daysLeft int, periodEnd time.Time) string {
	months := []string{"jan", "fév", "mar", "avr", "mai", "jun", "jul", "aoû", "sep", "oct", "nov", "déc"}
	dateStr := fmt.Sprintf("%02d %s %d", periodEnd.Day(), months[periodEnd.Month()-1], periodEnd.Year())

	bgColor := "#e8f0fe"
	textColor := "#007BFF"
	if daysLeft <= 2 {
		bgColor = "#fee2e2"
		textColor = "#ef4444"
	} else if daysLeft <= 5 {
		bgColor = "#fef3c7"
		textColor = "#f59e0b"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#f4f4f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f7;padding:40px 20px;">
  <tr><td align="center">
    <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;">
      <tr><td align="center" style="padding-bottom:24px;">
        <span style="font-size:22px;font-weight:700;color:#007BFF;">NEXIUM</span>
        <span style="font-size:14px;color:#999;margin-left:6px;">Storage</span>
      </td></tr>
      <tr><td style="background:#ffffff;border-radius:12px;box-shadow:0 2px 12px rgba(0,0,0,0.07);overflow:hidden;">
        <table width="100%%" cellpadding="0" cellspacing="0">
          <tr><td style="background:%s;padding:10px 28px;">
            <p style="margin:0;font-size:13px;font-weight:600;color:%s;">
              Expiration dans <strong>%d jour%s</strong> — %s
            </p>
          </td></tr>
        </table>
        <table width="100%%" cellpadding="0" cellspacing="0" style="padding:28px 32px;">
          <tr><td>
            <p style="margin:0 0 10px;font-size:15px;color:#1a1a2e;">Bonjour <strong>%s</strong>,</p>
            <p style="margin:0 0 20px;font-size:14px;color:#555;line-height:1.6;">
              Votre abonnement <strong>%s</strong> expire le <strong>%s</strong>.
              Sans renouvellement, votre compte repassera sur le plan <strong>Free</strong>
              (10 GB de stockage, fichiers ≤ 100 MB).
            </p>
            <table width="100%%" cellpadding="0" cellspacing="0">
              <tr><td align="center" style="padding-bottom:20px;">
                <a href="https://storage.nexium.ai/dashboard/billing"
                   style="display:inline-block;background:#007BFF;color:#fff;font-size:14px;font-weight:600;
                          text-decoration:none;padding:12px 28px;border-radius:8px;">
                  Renouveler mon abonnement
                </a>
              </td></tr>
            </table>
            <p style="margin:0;font-size:12px;color:#aaa;">
              Des questions ? Répondez à cet email ou écrivez-nous à support@nexium.ai.
            </p>
          </td></tr>
        </table>
      </td></tr>
      <tr><td align="center" style="padding-top:20px;">
        <p style="margin:0;font-size:12px;color:#bbb;">© %d NEXIUM Storage · Abidjan, Côte d'Ivoire</p>
      </td></tr>
    </table>
  </td></tr>
</table>
</body></html>`,
		bgColor, textColor,
		daysLeft, pluralS(daysLeft), dateStr,
		name, planName, dateStr,
		time.Now().Year(),
	)
}

// warnInactiveUsers envoie un email aux users inactifs depuis 90 jours non encore avertis.
func (j *Job) warnInactiveUsers(ctx context.Context) {
	rows, err := j.db.Query(ctx, `
		SELECT id, email, name FROM users
		WHERE last_active_at < now() - interval '90 days'
		  AND inactivity_warned_at IS NULL
		  AND last_active_at IS NOT NULL
	`)
	if err != nil {
		log.Printf("[expiry] warn query error: %v", err)
		return
	}
	defer rows.Close()

	var users []inactiveUser
	for rows.Next() {
		var u inactiveUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Name); err == nil {
			users = append(users, u)
		}
	}

	for _, u := range users {
		body := fmt.Sprintf(`
<p>Bonjour %s,</p>
<p>Votre compte <strong>%s</strong> est inactif depuis plus de 90 jours.</p>
<p>Sans activité dans les <strong>30 prochains jours</strong>, vos fichiers seront supprimés automatiquement.
Connectez-vous ou effectuez une action pour conserver vos données.</p>
<p>L'équipe %s</p>
`, u.Name, j.appName, j.appName)

		if err := j.mail.Send(ctx, u.Email, u.Name,
			fmt.Sprintf("[%s] Inactivité — vos fichiers seront supprimés dans 30 jours", j.appName),
			body,
		); err != nil {
			log.Printf("[expiry] warn email error user=%s: %v", u.ID, err)
			continue
		}

		j.db.Exec(ctx,
			`UPDATE users SET inactivity_warned_at = now() WHERE id = $1`, u.ID,
		)
		log.Printf("[expiry] warned user=%s", u.ID)
	}
}

// deleteExpiredFiles supprime tous les fichiers des users inactifs depuis 120 jours.
func (j *Job) deleteExpiredFiles(ctx context.Context) {
	rows, err := j.db.Query(ctx, `
		SELECT id FROM users
		WHERE last_active_at < now() - interval '120 days'
		  AND last_active_at IS NOT NULL
	`)
	if err != nil {
		log.Printf("[expiry] delete query error: %v", err)
		return
	}
	defer rows.Close()

	var userIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			userIDs = append(userIDs, id)
		}
	}

	for _, userID := range userIDs {
		if err := j.deleteUserFiles(ctx, userID); err != nil {
			log.Printf("[expiry] delete files error user=%s: %v", userID, err)
		}
	}
}

func (j *Job) deleteUserFiles(ctx context.Context, userID uuid.UUID) error {
	// Récupérer toutes les object_keys des fichiers de cet user
	rows, err := j.db.Query(ctx, `
		SELECT f.id, f.object_key
		FROM files f
		JOIN buckets b ON b.id = f.bucket_id
		JOIN projects p ON p.id = b.project_id
		WHERE p.user_id = $1
	`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type fileEntry struct {
		id        uuid.UUID
		objectKey string
	}
	var entries []fileEntry
	for rows.Next() {
		var e fileEntry
		if err := rows.Scan(&e.id, &e.objectKey); err == nil {
			entries = append(entries, e)
		}
	}
	rows.Close()

	if len(entries) == 0 {
		return nil
	}

	// Supprimer de R2 puis de la DB
	for _, e := range entries {
		if err := j.r2.Delete(ctx, e.objectKey); err != nil {
			log.Printf("[expiry] R2 delete error file=%s: %v", e.id, err)
		}
	}

	_, err = j.db.Exec(ctx, `
		DELETE FROM files f
		USING buckets b, projects p
		WHERE f.bucket_id = b.id
		  AND b.project_id = p.id
		  AND p.user_id = $1
	`, userID)
	if err != nil {
		return err
	}

	// Réinitialiser last_active_at pour éviter une re-suppression au prochain cycle
	j.db.Exec(ctx,
		`UPDATE users SET last_active_at = now(), inactivity_warned_at = NULL WHERE id = $1`,
		userID,
	)

	log.Printf("[expiry] deleted %d files for user=%s", len(entries), userID)
	return nil
}
