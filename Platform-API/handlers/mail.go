package handlers

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

// SendMail SMTP를 통해 메일 발송
func SendMail(to []string, subject, body string) error {
	if config.SMTPHost == "" {
		log.Printf("mail: SMTP 미설정, 메일 발송 생략 (to=%v subject=%s)", to, subject)
		return nil
	}

	from := config.SMTPFrom
	if from == "" {
		from = config.SMTPUser
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, strings.Join(to, ","), subject, body)

	addr := config.SMTPHost + ":" + config.SMTPPort
	var auth smtp.Auth
	if config.SMTPUser != "" {
		auth = smtp.PlainAuth("", config.SMTPUser, config.SMTPPass, config.SMTPHost)
	}

	if err := smtp.SendMail(addr, auth, from, to, []byte(msg)); err != nil {
		log.Printf("mail: 발송 실패 to=%v err=%v", to, err)
		return err
	}
	log.Printf("mail: 발송 완료 to=%v subject=%s", to, subject)
	return nil
}

// notifyPRCreated PR 생성 시 승인자들에게 알림 메일
func notifyPRCreated(repoName string, prNumber int, prTitle, prURL string, reviewers []string) {
	if len(reviewers) == 0 {
		return
	}
	// 이메일 주소가 아닌 경우 @도메인 추가 필요 — 현재는 로그인 ID만 저장되어 있으므로 로그로 남김
	emails := resolveEmails(reviewers)
	if len(emails) == 0 {
		log.Printf("mail: PR 생성 알림 — 이메일 주소를 확인할 수 없습니다 (reviewers=%v)", reviewers)
		return
	}

	subject := fmt.Sprintf("[GoJIRA] 승인 요청: %s (#%d)", prTitle, prNumber)
	body := fmt.Sprintf(`<h3>승인 요청</h3>
<p><strong>%s</strong> 레포지토리에 새로운 PR이 생성되었습니다.</p>
<ul>
  <li>PR: #%d %s</li>
  <li><a href="%s">GitHub에서 보기</a></li>
</ul>
<p>GoJIRA 문서 관리에서 승인/반려 처리해주세요.</p>`, repoName, prNumber, prTitle, prURL)

	go SendMail(emails, subject, body)
}

// notifyMergeable 머지 가능 상태 알림
func notifyMergeable(repoName string, prNumber int, prTitle, prURL, creator string) {
	emails := resolveEmails([]string{creator})
	if len(emails) == 0 {
		return
	}

	subject := fmt.Sprintf("[GoJIRA] 머지 가능: %s (#%d)", prTitle, prNumber)
	body := fmt.Sprintf(`<h3>머지 가능</h3>
<p><strong>%s</strong> 레포지토리의 PR이 모든 승인을 받았습니다.</p>
<ul>
  <li>PR: #%d %s</li>
  <li><a href="%s">GitHub에서 보기</a></li>
</ul>
<p>GoJIRA 문서 관리에서 머지를 진행해주세요.</p>`, repoName, prNumber, prTitle, prURL)

	go SendMail(emails, subject, body)
}

// resolveEmails 사용자 ID → 이메일 변환 (knoxId로 DB 조회)
func resolveEmails(users []string) []string {
	var emails []string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, u := range users {
		if strings.Contains(u, "@") {
			emails = append(emails, u)
			continue
		}
		// knoxId로 DB에서 이메일 조회
		user, err := db.FindUserByKnoxID(ctx, u)
		if err == nil {
			if email, _ := user["email"].(string); email != "" {
				emails = append(emails, email)
			}
		}
	}
	return emails
}
