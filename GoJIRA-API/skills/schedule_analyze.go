package skills

import (
	"context"
	"fmt"
	"time"

	"gojira-api/config"
	"gojira-api/db"
)

func init() {
	Register(&Skill{
		ID:          "schedule_analyze",
		Name:        "일정 분석",
		Description: "프로젝트의 마일스톤 일정을 분석하여 리스크와 진행 상태를 보고합니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"projectCode": { "type": "string", "description": "프로젝트 코드" }
			},
			"required": ["projectCode"]
		}`),
		Execute: executeScheduleAnalyze,
	})
}

func executeScheduleAnalyze(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode는 필수입니다")
	}

	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	prj := db.FindProjectByCode(qCtx, code)
	if prj == nil {
		return nil, fmt.Errorf("프로젝트를 찾을 수 없습니다: %s", code)
	}

	name, _ := prj["name"].(string)
	status, _ := prj["status"].(string)

	// 스케줄 추출
	type Milestone struct {
		Name string `json:"name"`
		Date string `json:"date"`
	}
	var milestones []Milestone
	if schedules, ok := prj["schedules"].([]interface{}); ok {
		for _, s := range schedules {
			switch sm := s.(type) {
			case map[string]interface{}:
				for k, v := range sm {
					if d, ok := v.(string); ok {
						milestones = append(milestones, Milestone{Name: k, Date: d})
					}
				}
			}
		}
	}

	// 이슈 현황
	issues, _ := db.FindAllIssues(qCtx, nil)
	var total, done, inProgress int
	for _, issue := range issues {
		key, _ := issue["key"].(string)
		if len(key) <= len(code)+1 {
			continue
		}
		if key[:len(code)] != code {
			continue
		}
		total++
		st, _ := issue["status"].(string)
		switch st {
		case "Done":
			done++
		case "In Progress":
			inProgress++
		}
	}

	// LLM 분석
	scheduleInfo := fmt.Sprintf("프로젝트: %s (%s)\n상태: %s\n마일스톤: %v\n이슈: 총 %d건 (완료 %d, 진행중 %d, 미착수 %d)\n오늘: %s",
		name, code, status, milestones, total, done, inProgress, total-done-inProgress, time.Now().Format("2006-01-02"))

	prompt := config.LoadPrompt("skill-schedule-analyze.txt", map[string]string{
		"ScheduleInfo": scheduleInfo,
	})
	if prompt == "" {
		prompt = fmt.Sprintf("다음 프로젝트 일정 정보를 분석하여 리스크, 진행률, 개선 제안을 한국어로 작성해주세요.\n\n%s", scheduleInfo)
	}

	analysis, err := CallLLM(prompt)
	if err != nil {
		// LLM 실패 시 기본 분석
		analysis = fmt.Sprintf("프로젝트 %s: 총 %d건 중 %d건 완료 (%.0f%%)", name, total, done, float64(done)/float64(max(total, 1))*100)
	}

	return map[string]interface{}{
		"projectCode": code,
		"projectName": name,
		"milestones":  milestones,
		"issueStats":  map[string]int{"total": total, "done": done, "inProgress": inProgress},
		"analysis":    analysis,
	}, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
