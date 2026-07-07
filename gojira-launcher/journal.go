package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 이슈 변경 저널 (D10 Phase 3 §옵션(a), web-local-view-model.md) — 웹 로컬뷰("내 작업본")
// 에서 발생한 이슈 변경(생성/수정/삭제/링크)을 그 과제의 에이전트 세션에 전달하기 위한
// 누적 로그. 클론 안(레포)이 아니라 런처 영역(~/.gojira/journal/<code>.yaml)에 둔다 —
// 프로젝트 레포에 .gitignore 가 없어 클론 안에 두면 PR 에 새어 들어가기 때문.
//
// 소비자: 에이전트 세션(spawn.go agentEnv 가 GOJIRA_ISSUE_JOURNAL env 로 경로를 주입).
// 에이전트가 SATAM/유스케이스/설계문서 영향을 검토한 뒤 처리한 항목을 파일에서 제거한다
// (제거 로직은 에이전트/스킬 쪽 책임 — 런처는 append 만 담당).

// JournalEntryInput — POST /issue-journal 요청 바디의 entry.
type JournalEntryInput struct {
	Op      string   `json:"op"`               // create|update|delete|link|unlink
	Keys    []string `json:"keys"`             // 영향받은 이슈 key(들)
	Summary string   `json:"summary"`          // 사람이 읽을 한 줄 요약
	Fields  []string `json:"fields,omitempty"` // update 시 변경된 필드명(선택)
}

// validJournalOps — entry.op 화이트리스트.
var validJournalOps = map[string]bool{
	"create": true,
	"update": true,
	"delete": true,
	"link":   true,
	"unlink": true,
}

// journalDir — ~/.gojira/journal 경로.
func journalDir() (string, error) {
	dir, err := gojiraDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "journal"), nil
}

// journalPath — ~/.gojira/journal/<code>.yaml 절대경로. code 는 호출측이 이미
// validateName 으로 검증했다고 가정(파일명에 그대로 쓰이므로).
func journalPath(code string) (string, error) {
	dir, err := journalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, code+".yaml"), nil
}

// journalLocks — code 별 mutex. 같은 과제로 거의 동시에 오는 두 웹 편집(예: 링크 생성이
// 양쪽 이슈에 각각 이벤트를 만드는 경우)이 파일 append 를 레이스 없이 순차 처리하도록.
// repoLocks(repo.go)와 분리된 이유: 대상 자원이 다르고(레포 vs 저널 파일), git 작업과
// 불필요하게 락을 공유하지 않기 위해.
var (
	journalLocks   = map[string]*sync.Mutex{}
	journalLocksMu sync.Mutex
)

func getJournalLock(code string) *sync.Mutex {
	journalLocksMu.Lock()
	defer journalLocksMu.Unlock()
	if journalLocks[code] == nil {
		journalLocks[code] = &sync.Mutex{}
	}
	return journalLocks[code]
}

// yamlQuote — 문자열을 안전한 YAML double-quoted scalar 로 직렬화한다.
// JSON 문자열 문법은 YAML double-quoted scalar 의 부분집합이라 encoding/json 으로 만든
// 결과를 그대로 재사용할 수 있다(따옴표/역슬래시/제어문자 이스케이프 동일). SetEscapeHTML(false)
// 로 <,>,& 를 \uXXXX 로 바꾸는 기본 동작을 꺼서 사람이 읽는 summary 가 원문 그대로 보이게 한다.
// 한글 등 비 ASCII 는 이스케이프 없이 raw UTF-8 로 남는다(JSON/YAML 모두 허용).
func yamlQuote(s string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", fmt.Errorf("yaml 문자열 직렬화 실패: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// formatJournalEntry — 저널 YAML 리스트의 항목 1개를 문자열로 만든다(개행으로 끝남).
// 모든 문자열 스칼라를 quote 하는 이유: op/user 등은 화이트리스트/패턴 검증을 거치지만,
// summary/keys/fields 는 임의 텍스트(콜론·한글·특수문자 포함 가능)라 항상 안전하게
// 직렬화해야 리스트 들여쓰기가 깨지지 않는다.
func formatJournalEntry(ts time.Time, user string, e JournalEntryInput) (string, error) {
	tsQ, err := yamlQuote(ts.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	userQ, err := yamlQuote(user)
	if err != nil {
		return "", err
	}
	opQ, err := yamlQuote(e.Op)
	if err != nil {
		return "", err
	}
	summaryQ, err := yamlQuote(e.Summary)
	if err != nil {
		return "", err
	}
	keysQ := make([]string, len(e.Keys))
	for i, k := range e.Keys {
		q, err := yamlQuote(k)
		if err != nil {
			return "", err
		}
		keysQ[i] = q
	}

	var b strings.Builder
	fmt.Fprintf(&b, "- ts: %s\n", tsQ)
	fmt.Fprintf(&b, "  user: %s\n", userQ)
	fmt.Fprintf(&b, "  op: %s\n", opQ)
	fmt.Fprintf(&b, "  keys: [%s]\n", strings.Join(keysQ, ", "))
	fmt.Fprintf(&b, "  summary: %s\n", summaryQ)
	if len(e.Fields) > 0 {
		fieldsQ := make([]string, len(e.Fields))
		for i, f := range e.Fields {
			q, err := yamlQuote(f)
			if err != nil {
				return "", err
			}
			fieldsQ[i] = q
		}
		fmt.Fprintf(&b, "  fields: [%s]\n", strings.Join(fieldsQ, ", "))
	}
	return b.String(), nil
}

// appendJournalEntry — ~/.gojira/journal/<code>.yaml 에 entry 1건을 append 한다.
// 파일 전체를 파싱/재작성하지 않는다 — YAML 리스트는 순서대로 append 만 해도 유효하므로
// append-only 로 충분(§구현 2). 디렉터리는 최초 호출 시 자동 생성.
func (s *Server) appendJournalEntry(code, user string, entry JournalEntryInput) (string, error) {
	path, err := journalPath(code)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("journal 디렉터리 생성 실패: %w", err)
	}
	block, err := formatJournalEntry(time.Now(), user, entry)
	if err != nil {
		return "", err
	}

	lock := getJournalLock(code)
	lock.Lock()
	defer lock.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("journal 파일 열기 실패: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return "", fmt.Errorf("journal 파일 쓰기 실패: %w", err)
	}
	return path, nil
}
