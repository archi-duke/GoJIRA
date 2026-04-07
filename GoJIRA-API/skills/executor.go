package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/config"
	"gojira-api/db"
)

// Execute Skill을 실행 (SKILL.md 정의 기반)
func (r *SkillRegistry) Execute(ctx context.Context, name string, params map[string]interface{}, user string) (interface{}, error) {
	skill := r.Get(name)
	if skill == nil {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return executeSkill(ctx, skill, params, user)
}

func executeSkill(ctx context.Context, skill *SkillDef, params map[string]interface{}, user string) (interface{}, error) {
	// action 기반 Skill인 경우
	if len(skill.Actions) > 0 {
		action, _ := params["action"].(string)
		if action == "" {
			return nil, fmt.Errorf("action 파라미터가 필요합니다")
		}
		apiDef, ok := skill.Actions[action]
		if !ok {
			return nil, fmt.Errorf("지원하지 않는 action: %s", action)
		}
		return executeAction(ctx, skill, &apiDef, action, params, user)
	}

	// 단일 API Skill인 경우
	if skill.API != nil {
		return executeSingleAPI(ctx, skill, params, user)
	}

	return nil, fmt.Errorf("skill %s에 api 또는 actions 정의가 없습니다", skill.Name)
}

func executeAction(ctx context.Context, skill *SkillDef, api *APIDef, action string, params map[string]interface{}, user string) (interface{}, error) {
	switch skill.Name {
	case "issue_manage":
		return executeIssueManage(ctx, api, action, params, user)
	case "doc_manage":
		return executeDocManage(ctx, api, action, params, user)
	case "pr_manage":
		return executePRManage(ctx, api, action, params, user)
	default:
		return executeGenericAPI(ctx, api, params, user)
	}
}

func executeSingleAPI(ctx context.Context, skill *SkillDef, params map[string]interface{}, user string) (interface{}, error) {
	switch skill.Name {
	case "project_query":
		return executeProjectQuery(ctx, params, user)
	case "issue_query":
		return executeIssueQuery(ctx, params, user)
	case "schedule_analyze":
		return executeScheduleAnalyze(ctx, skill, params, user)
	default:
		return executeGenericAPI(ctx, skill.API, params, user)
	}
}

// ─── 프로젝트 조회 ───

func executeProjectQuery(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	projects, err := db.FindAllProjects(qCtx, nil)
	if err != nil {
		return nil, err
	}
	filterCode, _ := params["projectCode"].(string)
	type P struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	var results []P
	for _, prj := range projects {
		code := bsonStr(bsonDoc(bsonDoc(prj, "jiraCodes"), "main"), "jiraCode")
		if code == "" {
			continue
		}
		if filterCode != "" && code != filterCode {
			continue
		}
		name, _ := prj["name"].(string)
		status, _ := prj["status"].(string)
		if status == "Deleted" {
			continue
		}
		results = append(results, P{Code: code, Name: name, Status: status})
	}
	return map[string]interface{}{"count": len(results), "projects": results}, nil
}

// ─── 이슈 조회 ───

func executeIssueQuery(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode는 필수입니다")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	filter := bson.M{"key": bson.M{"$regex": "^" + code + "-"}}
	if t, ok := params["issueType"].(string); ok && t != "" {
		filter["issueType"] = t
	}
	issues, err := db.FindAllIssues(qCtx, filter)
	if err != nil {
		return nil, err
	}
	type I struct {
		Key      string `json:"key"`
		Type     string `json:"issueType"`
		Summary  string `json:"summary"`
		Status   string `json:"status"`
		Assignee string `json:"assignee"`
	}
	var results []I
	for _, issue := range issues {
		k, _ := issue["key"].(string)
		t, _ := issue["issueType"].(string)
		s, _ := issue["summary"].(string)
		st, _ := issue["status"].(string)
		a, _ := issue["assignee"].(string)
		results = append(results, I{Key: k, Type: t, Summary: s, Status: st, Assignee: a})
	}
	return map[string]interface{}{"projectCode": code, "count": len(results), "issues": results}, nil
}

// ─── 이슈 관리 ───

func executeIssueManage(ctx context.Context, api *APIDef, action string, params map[string]interface{}, user string) (interface{}, error) {
	switch action {
	case "create":
		return issueCreate(ctx, params, user)
	case "update":
		return issueUpdate(ctx, params)
	case "delete":
		return issueDelete(ctx, params)
	case "link":
		return issueLink(ctx, params)
	default:
		return nil, fmt.Errorf("지원하지 않는 action: %s", action)
	}
}

func issueCreate(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	issueType, _ := params["issueType"].(string)
	summary, _ := params["summary"].(string)
	if code == "" || issueType == "" || summary == "" {
		return nil, fmt.Errorf("projectCode, issueType, summary는 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	nextNum := getNextIssueNum(qCtx, code)
	key := fmt.Sprintf("%s-%d", code, nextNum)
	desc, _ := params["description"].(string)
	pri, _ := params["priority"].(string)
	if pri == "" {
		pri = "Medium"
	}
	assignee, _ := params["assignee"].(string)
	doc := bson.M{
		"key": key, "issueType": issueType, "summary": summary, "description": desc,
		"status": "To Do", "priority": pri, "assignee": assignee, "reporter": user,
		"created": time.Now().Format("2006-01-02T15:04:05.000+0900"),
		"updated": time.Now().Format("2006-01-02T15:04:05.000+0900"),
		"labels": []string{}, "issueLinks": []interface{}{}, "_source": "gojira-agent",
	}
	if err := db.UpsertIssue(qCtx, key, doc); err != nil {
		return nil, err
	}
	log.Printf("Skill: issue create %s", key)
	return map[string]interface{}{"key": key, "issueType": issueType, "summary": summary, "status": "To Do"}, nil
}

func issueUpdate(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key는 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := db.FindIssueByKey(qCtx, key); err != nil {
		return nil, fmt.Errorf("이슈 없음: %s", key)
	}
	update := bson.M{"updated": time.Now().Format("2006-01-02T15:04:05.000+0900")}
	var changed []string
	for _, f := range []string{"summary", "description", "status", "priority", "assignee"} {
		if v, ok := params[f].(string); ok && v != "" {
			update[f] = v
			changed = append(changed, f)
		}
	}
	if len(changed) == 0 {
		return map[string]interface{}{"key": key, "message": "변경 없음"}, nil
	}
	db.UpsertIssue(qCtx, key, update)
	return map[string]interface{}{"key": key, "changed": changed}, nil
}

func issueDelete(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key는 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.DeleteIssueByKey(qCtx, key); err != nil {
		return nil, err
	}
	return map[string]interface{}{"key": key, "deleted": true}, nil
}

func issueLink(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	from, _ := params["fromKey"].(string)
	to, _ := params["toKey"].(string)
	lt, _ := params["linkType"].(string)
	if from == "" || to == "" || lt == "" {
		return nil, fmt.Errorf("fromKey, toKey, linkType 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	linkID := fmt.Sprintf("%s_%s_%s", from, lt, to)
	db.Issues().UpdateOne(qCtx, bson.M{"key": from},
		bson.M{"$addToSet": bson.M{"issueLinks": bson.M{"id": linkID, "type": lt, "outward": to}}})
	db.Issues().UpdateOne(qCtx, bson.M{"key": to},
		bson.M{"$addToSet": bson.M{"issueLinks": bson.M{"id": linkID, "type": lt, "inward": from}}})
	return map[string]interface{}{"linkId": linkID, "fromKey": from, "toKey": to, "linkType": lt}, nil
}

// ─── 문서 관리 ───

func executeDocManage(ctx context.Context, api *APIDef, action string, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode 필수")
	}
	switch action {
	case "list":
		return docList(code)
	case "summarize":
		return docSummarize(code, params, user)
	case "diff":
		return docDiff(code, params, user)
	default:
		return nil, fmt.Errorf("지원하지 않는 action: %s", action)
	}
}

func docList(code string) (interface{}, error) {
	resp, err := http.Get(config.PlatformAPI + fmt.Sprintf("/DocRepo/%s/tree?ref=main", code))
	if err != nil || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("파일 목록 조회 실패")
	}
	defer resp.Body.Close()
	var tree map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tree)
	items, _ := tree["tree"].([]interface{})
	var files []string
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if t, _ := m["type"].(string); t != "tree" {
			if p, _ := m["path"].(string); p != "" {
				files = append(files, p)
			}
		}
	}
	return map[string]interface{}{"projectCode": code, "files": files, "count": len(files)}, nil
}

func docSummarize(code string, params map[string]interface{}, user string) (interface{}, error) {
	fp, _ := params["filePath"].(string)
	if fp == "" {
		return nil, fmt.Errorf("filePath 필수")
	}
	content, err := FetchFileContent(code, fp, "main")
	if err != nil {
		content, err = FetchFileContent(code, fp, "docs/"+user)
		if err != nil {
			return nil, fmt.Errorf("파일 조회 실패")
		}
	}
	if content == "" {
		return map[string]interface{}{"summary": "파일이 비어 있습니다."}, nil
	}
	prompt := config.LoadPrompt("skill-doc-summarize.txt", map[string]string{"FilePath": fp, "Content": truncate(content, 8000)})
	if prompt == "" {
		prompt = fmt.Sprintf("다음 문서를 한국어로 요약:\n\n%s\n\n%s", fp, truncate(content, 8000))
	}
	summary, _ := CallLLM(prompt)
	return map[string]interface{}{"projectCode": code, "filePath": fp, "summary": summary}, nil
}

func docDiff(code string, params map[string]interface{}, user string) (interface{}, error) {
	fp, _ := params["filePath"].(string)
	if fp == "" {
		return nil, fmt.Errorf("filePath 필수")
	}
	mc, _ := FetchFileContent(code, fp, "main")
	uc, _ := FetchFileContent(code, fp, "docs/"+user)
	if mc == uc {
		return map[string]interface{}{"filePath": fp, "hasDiff": false, "summary": "차이 없음"}, nil
	}
	prompt := config.LoadPrompt("skill-diff-explain.txt", map[string]string{
		"FilePath": fp, "MainContent": truncate(mc, 4000), "UserContent": truncate(uc, 4000),
	})
	if prompt == "" {
		prompt = fmt.Sprintf("두 버전 비교:\n\n=== main ===\n%s\n\n=== user ===\n%s", truncate(mc, 4000), truncate(uc, 4000))
	}
	explanation, _ := CallLLM(prompt)
	return map[string]interface{}{"filePath": fp, "hasDiff": true, "explanation": explanation}, nil
}

// ─── PR 관리 ───

func executePRManage(ctx context.Context, api *APIDef, action string, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode 필수")
	}
	switch action {
	case "create":
		return prCreate(code, params, user)
	case "list":
		return prList(code, params)
	default:
		return nil, fmt.Errorf("지원하지 않는 action: %s", action)
	}
}

func prCreate(code string, params map[string]interface{}, user string) (interface{}, error) {
	title, _ := params["title"].(string)
	body, _ := params["body"].(string)
	if title == "" {
		title = fmt.Sprintf("[%s] 설계문서 배포 요청", code)
	}
	reqBody, _ := json.Marshal(map[string]string{"title": title, "body": body, "head": "docs/" + user, "base": "main"})
	req, _ := http.NewRequest("POST", config.PlatformAPI+fmt.Sprintf("/DocRepo/%s/pr", code), strings.NewReader(string(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", user)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PR 생성 실패: %s", string(data))
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return map[string]interface{}{"projectCode": code, "prNumber": result["number"], "prURL": result["html_url"], "title": title}, nil
}

func prList(code string, params map[string]interface{}) (interface{}, error) {
	state, _ := params["state"].(string)
	if state == "" {
		state = "open"
	}
	resp, err := http.Get(config.PlatformAPI + fmt.Sprintf("/DocRepo/%s/prs?state=%s", code, state))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var prs []interface{}
	json.NewDecoder(resp.Body).Decode(&prs)
	type PR struct {
		Num   int    `json:"number"`
		Title string `json:"title"`
		State string `json:"state"`
		User  string `json:"user"`
	}
	var results []PR
	for _, p := range prs {
		pm, _ := p.(map[string]interface{})
		n, _ := pm["number"].(float64)
		t, _ := pm["title"].(string)
		s, _ := pm["state"].(string)
		u := ""
		if um, ok := pm["user"].(map[string]interface{}); ok {
			u, _ = um["login"].(string)
		}
		results = append(results, PR{Num: int(n), Title: t, State: s, User: u})
	}
	return map[string]interface{}{"projectCode": code, "state": state, "count": len(results), "prs": results}, nil
}

// ─── 일정 분석 ───

func executeScheduleAnalyze(ctx context.Context, skill *SkillDef, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	prj := db.FindProjectByCode(qCtx, code)
	if prj == nil {
		return nil, fmt.Errorf("프로젝트 없음: %s", code)
	}
	name, _ := prj["name"].(string)
	status, _ := prj["status"].(string)
	// 이슈 통계
	issues, _ := db.FindAllIssues(qCtx, bson.M{"key": bson.M{"$regex": "^" + code + "-"}})
	var total, done, prog int
	for _, i := range issues {
		total++
		if s, _ := i["status"].(string); s == "Done" {
			done++
		} else if s == "In Progress" {
			prog++
		}
	}
	info := fmt.Sprintf("프로젝트: %s (%s), 상태: %s, 이슈: 총%d 완료%d 진행%d 미착수%d, 오늘: %s",
		name, code, status, total, done, prog, total-done-prog, time.Now().Format("2006-01-02"))

	prompt := config.LoadPrompt("skill-schedule-analyze.txt", map[string]string{"ScheduleInfo": info})
	if prompt == "" {
		prompt = "프로젝트 일정 분석:\n" + info
	}
	analysis, _ := CallLLM(prompt)
	return map[string]interface{}{
		"projectCode": code, "projectName": name,
		"issueStats": map[string]int{"total": total, "done": done, "inProgress": prog},
		"analysis":   analysis,
	}, nil
}

// ─── Generic API (확장용) ───

func executeGenericAPI(ctx context.Context, api *APIDef, params map[string]interface{}, user string) (interface{}, error) {
	return map[string]string{"message": "generic API execution not yet implemented"}, nil
}

// ─── bson 헬퍼 ───

func bsonDoc(doc interface{}, key string) interface{} {
	switch d := doc.(type) {
	case map[string]interface{}:
		return d[key]
	case bson.M:
		return d[key]
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				return e.Value
			}
		}
	}
	return nil
}

func bsonStr(doc interface{}, key string) string {
	switch d := doc.(type) {
	case map[string]interface{}:
		s, _ := d[key].(string)
		return s
	case bson.M:
		s, _ := d[key].(string)
		return s
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				s, _ := e.Value.(string)
				return s
			}
		}
	}
	return ""
}

func getNextIssueNum(ctx context.Context, code string) int {
	issues, _ := db.FindAllIssues(ctx, bson.M{"key": bson.M{"$regex": "^" + code + "-"}})
	max := 0
	prefix := code + "-"
	for _, i := range issues {
		k, _ := i["key"].(string)
		if len(k) > len(prefix) {
			var n int
			fmt.Sscanf(k[len(prefix):], "%d", &n)
			if n > max {
				max = n
			}
		}
	}
	return max + 1
}
