package handlers

import (
	"bytes"
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

var platformAPI = "http://localhost:9000/api/v2"

func init() {
	if v := config.PlatformAPI; v != "" {
		platformAPI = v
	}
}

// POST /Issue/docgen — 프로젝트 설계문서 생성/수정 + PR
func DocGenHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PrjID  string `json:"prjID"`
		UserID string `json:"userId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// 프로젝트 조회
	prjDoc := db.FindProjectByCode(ctx, req.PrjID)
	if prjDoc == nil {
		respondError(w, http.StatusNotFound, "프로젝트를 찾을 수 없습니다")
		return
	}

	// repo가 없으면 자동 생성 (Platform-API에 위임)
	repoName := getDocField(prjDoc, "repoName")
	log.Printf("docgen: %s repoName=%q documents_type=%T", req.PrjID, repoName, prjDoc["documents"])
	if repoName == "" {
		_, sc := callPlatform("POST", fmt.Sprintf("/DocRepo/%s", req.PrjID), nil)
		if sc >= 400 {
			respondError(w, http.StatusBadGateway, "설계문서 저장소 자동 생성 실패")
			return
		}
		// 프로젝트 다시 조회 (documents 갱신됨)
		prjDoc = db.FindProjectByCode(ctx, req.PrjID)
		repoName = getDocField(prjDoc, "repoName")
		if repoName == "" {
			respondError(w, http.StatusInternalServerError, "저장소 생성 후 정보를 가져올 수 없습니다")
			return
		}
	}

	// 프로젝트 auditors → Platform-API에 전달 (PR 생성 시 필요)
	if auditors := toStrSlice(prjDoc["auditors"]); len(auditors) > 0 {
		// auditors는 프로젝트에서 관리, Platform-API의 PR 생성에서 프로젝트에서 직접 읽음
	}

	// 열린 PR 확인
	openPRs, sc := callPlatform("GET", fmt.Sprintf("/DocRepo/%s/prs?state=open", req.PrjID), nil)
	if sc == 200 {
		if prList, ok := openPRs.([]interface{}); ok && len(prList) > 0 {
			titles := []string{}
			for _, pr := range prList {
				if m, ok := pr.(map[string]interface{}); ok {
					titles = append(titles, fmt.Sprintf("#%v %s", m["number"], m["title"]))
				}
			}
			respondError(w, http.StatusConflict,
				fmt.Sprintf("미완료된 PR이 %d건 있습니다.\n승인/머지를 완료하거나, 승인 진행 중이면 잠시 후 다시 시도하세요.\n\n%s",
					len(titles), strings.Join(titles, "\n")))
			return
		}
	}

	// 이슈 가져오기
	issueKeys := bson.A{}
	if arr, ok := prjDoc["issues"]; ok {
		switch v := arr.(type) {
		case bson.A:
			issueKeys = v
		case []interface{}:
			issueKeys = bson.A(v)
		}
	}
	if len(issueKeys) == 0 {
		respondError(w, http.StatusBadRequest, "프로젝트에 등록된 이슈가 없습니다")
		return
	}

	issues, err := db.FindAllIssues(ctx, bson.M{"key": bson.M{"$in": issueKeys}})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 이슈 타입별 분류 → Markdown 생성
	typeGroups := map[string][]bson.M{}
	for _, issue := range issues {
		it, _ := issue["issueType"].(string)
		typeGroups[it] = append(typeGroups[it], issue)
	}

	files := map[string]string{}
	projectName, _ := prjDoc["name"].(string)
	files["README.md"] = fmt.Sprintf("# %s 설계서\n\n프로젝트 코드: %s\n\n생성일: %s\n", projectName, req.PrjID, time.Now().Format("2006-01-02"))

	typeOrder := []string{"VOS", "Feature", "Requirement", "Architectural Design", "Detailed Design"}
	for _, typeName := range typeOrder {
		group := typeGroups[typeName]
		if len(group) == 0 {
			continue
		}
		fileName := strings.ReplaceAll(strings.ToLower(typeName), " ", "-") + ".md"
		md := fmt.Sprintf("# %s\n\n", typeName)
		for _, issue := range group {
			key, _ := issue["key"].(string)
			subkey, _ := issue["subkey"].(string)
			summary, _ := issue["summary"].(string)
			desc, _ := issue["description"].(string)
			md += fmt.Sprintf("## %s %s\n\n- Key: %s\n", subkey, summary, key)
			if desc != "" {
				md += fmt.Sprintf("\n%s\n", desc)
			}
			if typeName == "Requirement" {
				for _, f := range []string{"reqtype", "qa", "source", "stimulus", "artifact", "response", "measure"} {
					if v, ok := issue[f]; ok && v != nil && v != "" {
						md += fmt.Sprintf("- %s: %v\n", f, v)
					}
				}
			}
			if strings.Contains(typeName, "Design") {
				for _, f := range []string{"objectives", "decision", "pros", "cons", "risk", "sensitivity", "mitigation"} {
					if v, ok := issue[f]; ok && v != nil && v != "" {
						md += fmt.Sprintf("- %s: %v\n", f, v)
					}
				}
			}
			md += "\n---\n\n"
		}
		files[fileName] = md
	}

	// Platform-API /git/publish 호출 (projectCode 기반)
	userId := req.UserID
	if userId == "" {
		userId = "system"
	}
	branchName := fmt.Sprintf("docs/%s", userId)

	// LLM 기반 PR 제목/본문 생성
	prTitle, prBody := summarizePRContent(projectName, req.PrjID, files)

	publishResult, sc := callPlatform("POST", fmt.Sprintf("/DocRepo/%s/git/publish", req.PrjID),
		map[string]interface{}{
			"branch": branchName, "files": files,
			"message": prTitle, "prTitle": prTitle, "prBody": prBody,
		}, userId)
	if sc >= 400 {
		errMsg := "문서 배포 실패"
		if m, ok := publishResult.(map[string]interface{}); ok {
			if e, ok := m["error"].(string); ok {
				errMsg += ": " + e
			}
		}
		respondError(w, http.StatusBadGateway, errMsg)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "설계문서 생성 및 PR 요청 완료",
		"branch":  branchName,
		"files":   len(files),
		"result":  publishResult,
	})
}

func getDocField(prj bson.M, key string) string {
	switch docs := prj["documents"].(type) {
	case bson.M:
		s, _ := docs[key].(string)
		return s
	case bson.D:
		for _, e := range docs {
			if e.Key == key {
				s, _ := e.Value.(string)
				return s
			}
		}
	}
	return ""
}

func toStrSlice(v interface{}) []string {
	switch t := v.(type) {
	case bson.A:
		s := make([]string, 0, len(t))
		for _, item := range t {
			if str, ok := item.(string); ok {
				s = append(s, str)
			}
		}
		return s
	case []interface{}:
		s := make([]string, 0, len(t))
		for _, item := range t {
			if str, ok := item.(string); ok {
				s = append(s, str)
			}
		}
		return s
	case []string:
		return t
	}
	return nil
}

var platformClient = &http.Client{Timeout: 120 * time.Second}

func callPlatform(method, path string, body interface{}, userID ...string) (interface{}, int) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, platformAPI+path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if len(userID) > 0 && userID[0] != "" {
		req.Header.Set("X-User-Id", userID[0])
	}
	resp, err := platformClient.Do(req)
	if err != nil {
		return nil, 500
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var result interface{}
	json.Unmarshal(data, &result)
	return result, resp.StatusCode
}
