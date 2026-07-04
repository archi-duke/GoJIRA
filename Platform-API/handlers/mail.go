package handlers

import (
	"fmt"

	"platform-api/notify"
)

// PR 이벤트 → 채널 중립 알림 조립. 실제 발송 채널(SMTP/사내 API/메신저)은
// notify 패키지가 기동 시 설정(NOTIFY_PROVIDER)으로 연결한 provider 가 결정한다.

// notifyPRCreated PR 생성 시 승인자들에게 승인 요청 알림
func notifyPRCreated(repoName string, prNumber int, prTitle, prURL string, reviewers []string) {
	if len(reviewers) == 0 {
		return
	}
	go notify.Send(notify.Message{
		Kind:  notify.KindPRCreated,
		To:    reviewers,
		Title: fmt.Sprintf("[GoJIRA] 승인 요청: %s (#%d)", prTitle, prNumber),
		Body: fmt.Sprintf("%s 레포지토리에 새로운 PR(#%d %s)이 생성되었습니다. GoJIRA 문서 관리에서 승인/반려 처리해주세요.",
			repoName, prNumber, prTitle),
		Link: prURL,
		Meta: map[string]string{
			"repo":     repoName,
			"prNumber": fmt.Sprintf("%d", prNumber),
		},
	})
}

// notifyMergeable 전원 승인 완료 시 PR 생성자에게 머지 가능 알림
func notifyMergeable(repoName string, prNumber int, prTitle, prURL, creator string) {
	go notify.Send(notify.Message{
		Kind:  notify.KindPRMergeable,
		To:    []string{creator},
		Title: fmt.Sprintf("[GoJIRA] 머지 가능: %s (#%d)", prTitle, prNumber),
		Body: fmt.Sprintf("%s 레포지토리의 PR(#%d %s)이 모든 승인을 받았습니다. GoJIRA 문서 관리에서 머지를 진행해주세요.",
			repoName, prNumber, prTitle),
		Link: prURL,
		Meta: map[string]string{
			"repo":     repoName,
			"prNumber": fmt.Sprintf("%d", prNumber),
		},
	})
}
