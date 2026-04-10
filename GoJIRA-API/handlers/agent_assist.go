package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"gojira-api/skills"
)

type assistRequest struct {
	Action      string                 `json:"action"`  // generate, review, suggest, explain
	Context     map[string]interface{} `json:"context"` // page, projectCode, issueKey, filePath, etc.
	Instruction string                 `json:"instruction"`
}

type assistResponse struct {
	Result  interface{}              `json:"result"`
	Actions []map[string]interface{} `json:"actions"`
	Preview bool                     `json:"preview"` // true면 사용자 확인 후 적용
}

// POST /api/v{version}/Agent/assist
func AgentAssistHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		userID = "anonymous"
	}

	var req assistRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	page, _ := req.Context["page"].(string)
	projectCode, _ := req.Context["projectCode"].(string)

	log.Printf("Agent Assist: action=%s page=%s project=%s user=%s", req.Action, page, projectCode, userID)

	// 페이지 + 액션 → Skill/LLM 호출 조합
	result, actions, err := routeAssist(ctx, req, projectCode, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, assistResponse{
		Result:  result,
		Actions: actions,
		Preview: req.Action == "generate", // 생성 액션은 미리보기
	})
}

func routeAssist(ctx context.Context, req assistRequest, projectCode, user string) (interface{}, []map[string]interface{}, error) {
	page, _ := req.Context["page"].(string)

	switch page {
	case "issues":
		return assistIssues(ctx, req, projectCode, user)
	case "docs":
		return assistDocs(ctx, req, projectCode, user)
	default:
		return assistGeneral(ctx, req, projectCode, user)
	}
}

// Issues 화면 Assist
func assistIssues(ctx context.Context, req assistRequest, projectCode, user string) (interface{}, []map[string]interface{}, error) {
	issueType, _ := req.Context["issueType"].(string)

	switch req.Action {
	case "generate":
		// 이슈 자동 생성: LLM에게 프로젝트 컨텍스트를 주고 이슈 제안 받기
		return generateIssues(ctx, projectCode, issueType, req.Instruction, user)
	case "suggest":
		// 관련 이슈/필드 추천
		return suggestForIssue(ctx, req.Context, user)
	default:
		return map[string]string{"message": "지원하지 않는 액션입니다"}, nil, nil
	}
}

// Docs 화면 Assist (문서 생성/수정 스킬은 보류 — 기본 기능만)
func assistDocs(ctx context.Context, req assistRequest, projectCode, user string) (interface{}, []map[string]interface{}, error) {
	filePath, _ := req.Context["filePath"].(string)

	switch req.Action {
	case "review":
		result, err := skills.Default.Execute(ctx, "doc_manage",
			map[string]interface{}{"action": "summarize", "projectCode": projectCode, "filePath": filePath}, user)
		if err != nil {
			return nil, nil, err
		}
		return result, []map[string]interface{}{{"skill": "doc_manage", "action": "summarize"}}, nil
	case "explain":
		result, err := skills.Default.Execute(ctx, "doc_manage",
			map[string]interface{}{"action": "diff", "projectCode": projectCode, "filePath": filePath}, user)
		if err != nil {
			return nil, nil, err
		}
		return result, []map[string]interface{}{{"skill": "doc_manage", "action": "diff"}}, nil
	default:
		return map[string]string{"message": "문서 생성/수정 기능은 준비 중입니다"}, nil, nil
	}
}

// 일반 Assist
func assistGeneral(ctx context.Context, req assistRequest, projectCode, user string) (interface{}, []map[string]interface{}, error) {
	return map[string]string{"message": "이 페이지에서는 Agent Assist가 지원되지 않습니다. 채팅을 이용해주세요."}, nil, nil
}

// generateIssues 프로젝트 컨텍스트 기반 이슈 자동 생성 제안
func generateIssues(ctx context.Context, projectCode, issueType, instruction, user string) (interface{}, []map[string]interface{}, error) {
	if projectCode == "" {
		return nil, nil, fmt.Errorf("projectCode는 필수입니다")
	}

	// 기존 이슈 조회
	existingResult, err := skills.Default.Execute(ctx, "issue_query",
		map[string]interface{}{"projectCode": projectCode}, user)
	if err != nil {
		return nil, nil, err
	}

	// project-description.md 내용 조회 (VOS 생성 시 참고)
	businessContent := ""
	if issueType == "VOS" || issueType == "" {
		bc, _ := skills.FetchFileContent(projectCode, "business/project-description.md", "main")
		if bc == "" {
			bc, _ = skills.FetchFileContent(projectCode, "business/project-description.md", "docs/"+user)
		}
		businessContent = bc
	}

	existingJSON, _ := json.Marshal(existingResult)

	prompt := fmt.Sprintf(`프로젝트 %s의 이슈를 분석하여 %s 타입의 새 이슈를 제안해주세요.

기존 이슈:
%s

%s

%s

규칙:
- 순수 JSON 배열만 응답하세요
- 각 항목: {"issueType": "...", "summary": "...", "description": "..."}
- 기존 이슈와 중복되지 않도록 하세요
- 3~5개 제안하세요`,
		projectCode,
		issueTypeLabel(issueType),
		truncateStr(string(existingJSON), 3000),
		businessContextStr(businessContent),
		instructionStr(instruction))

	reply, err := skills.CallLLM(prompt)
	if err != nil {
		return nil, nil, fmt.Errorf("LLM 호출 실패: %w", err)
	}

	// JSON 배열 파싱
	var suggestions []map[string]interface{}
	if err := json.Unmarshal([]byte(reply), &suggestions); err != nil {
		// 코드블록으로 감싼 경우
		cleaned := cleanJSON(reply)
		json.Unmarshal([]byte(cleaned), &suggestions)
	}

	return map[string]interface{}{
		"projectCode": projectCode,
		"issueType":   issueType,
		"suggestions":  suggestions,
		"count":        len(suggestions),
	}, nil, nil
}

func suggestForIssue(ctx context.Context, ctxMap map[string]interface{}, user string) (interface{}, []map[string]interface{}, error) {
	return map[string]string{"message": "이슈 추천 기능은 준비 중입니다"}, nil, nil
}

func issueTypeLabel(t string) string {
	if t == "" {
		return "전체"
	}
	return t
}

func businessContextStr(content string) string {
	if content == "" {
		return ""
	}
	return fmt.Sprintf("비즈니스 문서(project-description.md):\n%s", truncateStr(content, 2000))
}

func instructionStr(inst string) string {
	if inst == "" {
		return ""
	}
	return fmt.Sprintf("사용자 지시: %s", inst)
}

func truncateStr(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func cleanJSON(s string) string {
	// ```json ... ``` 제거
	start := 0
	if i := indexOf(s, "```"); i >= 0 {
		if nl := indexOf(s[i:], "\n"); nl >= 0 {
			start = i + nl + 1
		}
	}
	end := len(s)
	if i := lastIndexOf(s, "```"); i > start {
		end = i
	}
	return s[start:end]
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func lastIndexOf(s, sub string) int {
	last := -1
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			last = i
		}
	}
	return last
}
