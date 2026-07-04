package notify

import (
	"log"

	"platform-api/config"
)

// internalNotifier 사내 환경용 — 별도 제공되는 사내 알림 API 서비스 연동.
//
// TODO(사내반영): 사내 API 스펙 확정 후 Send 를 구현한다. 이 파일만 채우면 되고
// 발송처(핸들러)·인터페이스는 변경 불필요. 예상 작업:
//   1. config.NotifyAPIURL / NotifyAPIToken 으로 HTTP 클라이언트 구성
//   2. Message → 사내 API 요청 페이로드 매핑 (Kind 별 템플릿 분기 가능)
//   3. msg.To(knoxId) 를 사내 API 가 요구하는 수신자 식별자로 변환
//   4. 메신저 등 채널 추가 시 Kind/Meta 로 라우팅
type internalNotifier struct{}

func newInternalNotifier() Notifier { return internalNotifier{} }

func (i internalNotifier) Name() string {
	if config.NotifyAPIURL != "" {
		return "internal (" + config.NotifyAPIURL + ", 미구현 스텁)"
	}
	return "internal (미구현 스텁)"
}

func (i internalNotifier) Send(msg Message) error {
	// 스펙 확정 전 스텁 — 발송하지 않고 호출 사실만 로그로 남긴다.
	log.Printf("[notify] internal: 사내 알림 API 미구현 — 발송 생략 (kind=%s to=%v title=%s link=%s)",
		msg.Kind, msg.To, msg.Title, msg.Link)
	return nil
}
