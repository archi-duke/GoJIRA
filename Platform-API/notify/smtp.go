package notify

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"platform-api/config"
	"platform-api/db"
)

// smtpNotifier 사외/dev 환경용 — SMTP 로 HTML 메일 발송.
// 수신자 ID(knoxId) → 이메일 변환도 메일 채널 고유 책임이므로 여기서 수행한다.
type smtpNotifier struct{}

func newSMTPNotifier() Notifier { return smtpNotifier{} }

func (s smtpNotifier) Name() string {
	return fmt.Sprintf("smtp (%s:%s)", config.SMTPHost, config.SMTPPort)
}

func (s smtpNotifier) Send(msg Message) error {
	emails := resolveEmails(msg.To)
	if len(emails) == 0 {
		log.Printf("[notify] smtp: 이메일 주소를 확인할 수 없습니다 (kind=%s to=%v)", msg.Kind, msg.To)
		return nil
	}

	from := config.SMTPFrom
	if from == "" {
		from = config.SMTPUser
	}

	raw := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, strings.Join(emails, ","), msg.Title, renderMailBody(msg))

	addr := config.SMTPHost + ":" + config.SMTPPort
	var auth smtp.Auth
	if config.SMTPUser != "" {
		auth = smtp.PlainAuth("", config.SMTPUser, config.SMTPPass, config.SMTPHost)
	}

	if err := smtp.SendMail(addr, auth, from, emails, []byte(raw)); err != nil {
		log.Printf("[notify] smtp: 발송 실패 to=%v err=%v", emails, err)
		return err
	}
	log.Printf("[notify] smtp: 발송 완료 to=%v subject=%s", emails, msg.Title)
	return nil
}

// renderMailBody 채널 중립 Message 를 HTML 메일 본문으로 조립
func renderMailBody(msg Message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<h3>%s</h3>\n<p>%s</p>\n", msg.Title, msg.Body)
	if msg.Link != "" {
		fmt.Fprintf(&b, `<p><a href="%s">바로가기</a></p>`+"\n", msg.Link)
	}
	return b.String()
}

// resolveEmails 사용자 ID → 이메일 변환 (knoxId 로 DB 조회, 이미 이메일이면 그대로)
func resolveEmails(users []string) []string {
	var emails []string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, u := range users {
		if strings.Contains(u, "@") {
			emails = append(emails, u)
			continue
		}
		user, err := db.FindUserByKnoxID(ctx, u)
		if err == nil {
			if email, _ := user["email"].(string); email != "" {
				emails = append(emails, email)
			}
		}
	}
	return emails
}
