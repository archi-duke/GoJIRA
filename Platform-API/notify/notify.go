// Package notify — 알림 발송 표준 인터페이스와 provider 로딩.
//
// 서비스 기동 시 Init() 이 NOTIFY_PROVIDER 설정에 따라 구현체를 선택해 연결한다.
//   - "smtp"     : 사외/dev 환경 — SMTP 메일 직접 발송 (smtp.go)
//   - "internal" : 사내 환경 — 별도 제공되는 사내 알림 API (internal.go, 스펙 확정 전 스텁)
//   - "none"     : 발송 비활성 (로그만)
//   - 미설정     : SMTP_HOST 가 있으면 smtp, 없으면 none 으로 자동 판별
//
// 인터페이스는 메일에 한정하지 않는다 — 메신저/사내 알림 등 채널 확장을 감안해
// 채널 중립 Message 를 주고받으며, 채널별 표현(HTML 메일 본문, 메신저 텍스트 등)은
// 각 구현체가 Message 필드에서 스스로 조립한다. 발송처(핸들러)는 구현체를 알 필요
// 없이 notify.Send() 만 호출한다.
package notify

import (
	"fmt"
	"log"

	"platform-api/config"
)

// 알림 종류 — 채널별 구현이 템플릿/라우팅 분기에 사용한다.
const (
	KindPRCreated   = "pr_created"   // PR 생성 → 승인자에게 승인 요청
	KindPRMergeable = "pr_mergeable" // 전원 승인 → 생성자에게 머지 가능
)

// Message 채널 중립 알림 메시지.
// 메일은 Subject/BodyHTML 을 그대로 쓰고, 제목 개념이 없는 채널(메신저 등)은
// Title 을 본문 첫 줄로 합치는 식으로 각 구현체가 재해석한다.
type Message struct {
	Kind  string            // 알림 종류 (Kind* 상수) — 템플릿/라우팅 분기용
	To    []string          // 수신자 — 사용자 ID(knoxId) 기준. 채널별 주소 변환은 구현체 책임
	Title string            // 알림 제목 (채널 중립 — 메일 Subject, 메신저 첫 줄 등)
	Body  string            // 본문 텍스트 (채널 중립, plain text)
	Link  string            // 관련 링크 (PR URL 등) — 채널별로 버튼/앵커/꼬리말로 표현
	Meta  map[string]string // 채널별 부가 정보 (repo, prNumber 등)
}

// Notifier 알림 발송 표준 인터페이스. 새 채널(사내 메신저 등)은 이 인터페이스를
// 구현하고 Init() 의 provider 분기에 등록하면 된다.
type Notifier interface {
	// Name provider 식별자 (기동 로그/진단용)
	Name() string
	// Send 채널 중립 메시지를 채널별 표현으로 변환해 발송
	Send(msg Message) error
}

var active Notifier = noopNotifier{reason: "Init() 미호출"}

// Init 설정에 따라 Notifier 구현체를 선택해 연결한다. main() 기동 시 1회 호출.
// 명시 설정이 잘못된 경우 fail-fast (silent fallback 방지 — config 철학과 동일).
func Init() {
	switch config.NotifyProvider {
	case "smtp":
		if config.SMTPHost == "" {
			log.Fatalf("[notify] NOTIFY_PROVIDER=smtp 이지만 SMTP_HOST 미설정 — .env 확인 필요")
		}
		active = newSMTPNotifier()
	case "internal":
		active = newInternalNotifier()
	case "none":
		active = noopNotifier{reason: "NOTIFY_PROVIDER=none"}
	case "":
		// 자동 판별 — 기존 동작 호환 (SMTP 설정이 있으면 발송, 없으면 생략)
		if config.SMTPHost != "" {
			active = newSMTPNotifier()
		} else {
			active = noopNotifier{reason: "NOTIFY_PROVIDER/SMTP_HOST 미설정"}
		}
	default:
		log.Fatalf("[notify] 알 수 없는 NOTIFY_PROVIDER: %q (smtp|internal|none)", config.NotifyProvider)
	}
	log.Printf("[notify] provider 연결: %s", active.Name())
}

// Provider 현재 연결된 provider 이름 (기동 배너/진단용)
func Provider() string {
	return active.Name()
}

// Send 현재 연결된 provider 로 알림 발송
func Send(msg Message) error {
	return active.Send(msg)
}

// noopNotifier 발송 비활성 — 호출을 로그로만 남긴다.
type noopNotifier struct{ reason string }

func (n noopNotifier) Name() string { return fmt.Sprintf("none (%s)", n.reason) }

func (n noopNotifier) Send(msg Message) error {
	log.Printf("[notify] 발송 생략(%s) kind=%s to=%v title=%s", n.reason, msg.Kind, msg.To, msg.Title)
	return nil
}
