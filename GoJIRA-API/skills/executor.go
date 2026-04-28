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
	switch skill.Name {
	// 조회
	case "project_query":
		return execProjectQuery(ctx, params)
	case "issue_query":
		return execIssueQuery(ctx, params)
	case "doc_list":
		return execDocList(params)
	case "pr_list":
		return execPRList(params)
	case "doc_history":
		return execDocHistory(params)
	case "satam_analyze":
		return execSATAMAnalyze(ctx, params)
	// 변경
	case "issue_create":
		return execIssueCreate(ctx, params, user)
	case "issue_update":
		return execIssueUpdate(ctx, params)
	case "issue_delete":
		return execIssueDelete(ctx, params)
	case "issue_link":
		return execIssueLink(ctx, params)
	case "pr_create":
		return execPRCreate(params, user)
	default:
		return nil, fmt.Errorf("executor not implemented for skill: %s", skill.Name)
	}
}

// ─── 조회 Skill ───

func execProjectQuery(ctx context.Context, params map[string]interface{}) (interface{}, error) {
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
		if code == "" || (filterCode != "" && code != filterCode) {
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

func execIssueQuery(ctx context.Context, params map[string]interface{}) (interface{}, error) {
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
	// reqType이 지정되면 issueType을 Requirement로 자동 설정하고 reqtype 필터 추가
	if rt, ok := params["reqType"].(string); ok && rt != "" {
		filter["issueType"] = "Requirement"
		filter["reqtype"] = rt
	}
	issues, err := db.FindAllIssues(qCtx, filter)
	if err != nil {
		return nil, err
	}
	type I struct {
		Key      string `json:"key"`
		Type     string `json:"issueType"`
		ReqType  string `json:"reqType,omitempty"`
		Summary  string `json:"summary"`
		Status   string `json:"status"`
		Assignee string `json:"assignee"`
	}
	var results []I
	for _, issue := range issues {
		results = append(results, I{
			Key: str(issue, "key"), Type: str(issue, "issueType"), ReqType: str(issue, "reqtype"),
			Summary: str(issue, "summary"), Status: str(issue, "status"), Assignee: str(issue, "assignee"),
		})
	}
	return map[string]interface{}{"projectCode": code, "count": len(results), "issues": results}, nil
}

func execDocList(params map[string]interface{}) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode 필수")
	}
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

func execPRList(params map[string]interface{}) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	state, _ := params["state"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode 필수")
	}
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

func execDocHistory(params map[string]interface{}) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode 필수")
	}
	branch, _ := params["branch"].(string)
	if branch == "" {
		branch = "main"
	}
	limit := "20"
	if l, ok := params["limit"].(string); ok && l != "" {
		limit = l
	}
	resp, err := http.Get(config.PlatformAPI + fmt.Sprintf("/DocRepo/%s/commits?branch=%s&perPage=%s", code, branch, limit))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var commits []interface{}
	json.NewDecoder(resp.Body).Decode(&commits)
	type C struct {
		SHA     string `json:"sha"`
		Message string `json:"message"`
		Author  string `json:"author"`
		Date    string `json:"date"`
	}
	var results []C
	for _, c := range commits {
		cm, _ := c.(map[string]interface{})
		commit, _ := cm["commit"].(map[string]interface{})
		author, _ := commit["author"].(map[string]interface{})
		sha, _ := cm["sha"].(string)
		if len(sha) > 7 {
			sha = sha[:7]
		}
		msg, _ := commit["message"].(string)
		if idx := strings.Index(msg, "\n"); idx > 0 {
			msg = msg[:idx]
		}
		name, _ := author["name"].(string)
		date, _ := author["date"].(string)
		results = append(results, C{SHA: sha, Message: msg, Author: name, Date: date})
	}
	return map[string]interface{}{"projectCode": code, "branch": branch, "count": len(results), "commits": results}, nil
}

// ─── 변경 Skill ───

func execIssueCreate(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	issueType, _ := params["issueType"].(string)
	summary, _ := params["summary"].(string)
	if code == "" || issueType == "" || summary == "" {
		return nil, fmt.Errorf("projectCode, issueType, summary 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	key := fmt.Sprintf("%s-%d", code, getNextIssueNum(qCtx, code))
	desc, _ := params["description"].(string)
	pri, _ := params["priority"].(string)
	if pri == "" {
		pri = "Medium"
	}
	assignee, _ := params["assignee"].(string)
	reqType, _ := params["reqType"].(string)
	doc := bson.M{
		"key": key, "issueType": issueType, "summary": summary, "description": desc,
		"status": "To Do", "priority": pri, "assignee": assignee, "reporter": user,
		"created": time.Now().Format("2006-01-02T15:04:05.000+0900"),
		"updated": time.Now().Format("2006-01-02T15:04:05.000+0900"),
		"labels": []string{}, "issueLinks": []interface{}{}, "_source": "gojira-agent",
	}
	if reqType != "" {
		doc["reqtype"] = reqType
	}
	if err := db.UpsertIssue(qCtx, key, doc); err != nil {
		return nil, err
	}
	log.Printf("Skill: issue_create %s (%s)", key, issueType)
	return map[string]interface{}{"key": key, "issueType": issueType, "summary": summary, "status": "To Do"}, nil
}

func execIssueUpdate(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := db.FindIssueByKey(qCtx, key); err != nil {
		return nil, fmt.Errorf("이슈 없음: %s", key)
	}
	update := bson.M{"updated": time.Now().Format("2006-01-02T15:04:05.000+0900")}
	var changed []string
	// reqType은 별도 처리 (MongoDB 필드명이 reqtype)
	if rt, ok := params["reqType"].(string); ok && rt != "" {
		update["reqtype"] = rt
		changed = append(changed, "reqType")
	}
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
	log.Printf("Skill: issue_update %s %v", key, changed)
	return map[string]interface{}{"key": key, "changed": changed}, nil
}

func execIssueDelete(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.DeleteIssueByKey(qCtx, key); err != nil {
		return nil, err
	}
	log.Printf("Skill: issue_delete %s", key)
	return map[string]interface{}{"key": key, "deleted": true}, nil
}

func execIssueLink(ctx context.Context, params map[string]interface{}) (interface{}, error) {
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
	log.Printf("Skill: issue_link %s → %s (%s)", from, to, lt)
	return map[string]interface{}{"linkId": linkID, "fromKey": from, "toKey": to, "linkType": lt}, nil
}

func execPRCreate(params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode 필수")
	}
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
	log.Printf("Skill: pr_create %s #%v", code, result["number"])
	return map[string]interface{}{"projectCode": code, "prNumber": result["number"], "prURL": result["html_url"], "title": title}, nil
}

// ─── SATAM 분석 ───

func execSATAMAnalyze(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode 필수")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// QA/NFR 이슈
	qaIssues, _ := db.FindAllIssues(qCtx, bson.M{
		"key": bson.M{"$regex": "^" + code + "-"}, "issueType": "Requirement",
	})
	// ADSN 이슈
	adsnIssues, _ := db.FindAllIssues(qCtx, bson.M{
		"key": bson.M{"$regex": "^" + code + "-"}, "issueType": "Architectural Design",
	})

	type SATAMItem struct {
		Key         string `json:"key"`
		Summary     string `json:"summary"`
		ReqType     string `json:"reqType,omitempty"`
		Risk        string `json:"risk,omitempty"`
		Sensitivity string `json:"sensitivity,omitempty"`
		Mitigation  string `json:"mitigation,omitempty"`
		IssueType   string `json:"issueType"`
	}

	var items []SATAMItem
	var riskCount, sensCount, mitigMissing int

	allIssues := append(qaIssues, adsnIssues...)
	for _, issue := range allIssues {
		risk := str(issue, "risk")
		sens := str(issue, "sensitivity")
		mitig := str(issue, "mitigation")
		if risk != "" {
			riskCount++
		}
		if sens != "" {
			sensCount++
		}
		if (risk != "" || sens != "") && mitig == "" {
			mitigMissing++
		}
		items = append(items, SATAMItem{
			Key: str(issue, "key"), Summary: str(issue, "summary"),
			ReqType: str(issue, "reqtype"), Risk: risk, Sensitivity: sens,
			Mitigation: mitig, IssueType: str(issue, "issueType"),
		})
	}

	// LLM 분석
	itemsJSON, _ := json.Marshal(items)
	prompt := fmt.Sprintf(`다음 프로젝트 %s의 SATAM 분석 데이터를 한국어로 분석해주세요.

데이터:
%s

분석 항목:
1. 위험도(risk) 있는 항목 목록과 심각도
2. 민감도(sensitivity) 있는 항목 목록
3. 경감조치(mitigation) 미완료 항목 (위험/민감인데 경감조치 없음)
4. 전체 커버리지 요약
5. 개선 권고사항`, code, truncate(string(itemsJSON), 6000))

	analysis, _ := CallLLM(prompt)

	return map[string]interface{}{
		"projectCode":    code,
		"totalItems":     len(items),
		"riskCount":      riskCount,
		"sensitiveCount": sensCount,
		"mitigMissing":   mitigMissing,
		"items":          items,
		"analysis":       analysis,
	}, nil
}

func toMapI(v interface{}) map[string]interface{} {
	switch m := v.(type) {
	case map[string]interface{}:
		return m
	case bson.M:
		return map[string]interface{}(m)
	case bson.D:
		r := map[string]interface{}{}
		for _, e := range m {
			r[e.Key] = e.Value
		}
		return r
	}
	return nil
}

// ─── 헬퍼 ───

func str(m bson.M, key string) string {
	s, _ := m[key].(string)
	return s
}

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
	mx := 0
	prefix := code + "-"
	for _, i := range issues {
		k, _ := i["key"].(string)
		if len(k) > len(prefix) {
			var n int
			fmt.Sscanf(k[len(prefix):], "%d", &n)
			if n > mx {
				mx = n
			}
		}
	}
	return mx + 1
}
