package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		PrjID     string `json:"prjID"`
		DocRepoID string `json:"docRepoId"`
		Message   string `json:"message"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// 프로젝트의 이슈 가져오기
	prjDoc := db.FindProjectByCode(ctx, req.PrjID)
	if prjDoc == nil {
		respondError(w, http.StatusNotFound, "프로젝트를 찾을 수 없습니다")
		return
	}

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

	// 이슈 타입별 분류
	typeGroups := map[string][]bson.M{}
	for _, issue := range issues {
		it, _ := issue["issueType"].(string)
		typeGroups[it] = append(typeGroups[it], issue)
	}

	// Markdown 문서 생성
	files := map[string]string{}

	// README.md
	projectName, _ := prjDoc["Name"].(string)
	files["README.md"] = fmt.Sprintf("# %s 설계서\n\n프로젝트 코드: %s\n\n생성일: %s\n", projectName, req.PrjID, time.Now().Format("2006-01-02"))

	// 타입별 문서
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

			md += fmt.Sprintf("## %s %s\n\n", subkey, summary)
			md += fmt.Sprintf("- Key: %s\n", key)
			if desc != "" {
				md += fmt.Sprintf("\n%s\n", desc)
			}

			// Requirement 추가 필드
			if typeName == "Requirement" {
				for _, f := range []string{"reqtype", "qa", "source", "stimulus", "artifact", "response", "measure"} {
					if v, ok := issue[f]; ok && v != nil && v != "" {
						md += fmt.Sprintf("- %s: %v\n", f, v)
					}
				}
			}

			// Design 추가 필드
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

	// 로컬 git 방식: 브랜치 생성 + 파일 쓰기 + commit/push + PR 생성
	branchName := fmt.Sprintf("docs/%s-%s", req.PrjID, time.Now().Format("20060102-150405"))
	prTitle := fmt.Sprintf("[%s] 설계문서 갱신 %s", req.PrjID, time.Now().Format("2006-01-02"))
	prBody := fmt.Sprintf("프로젝트 %s의 설계문서를 갱신합니다.\n\n변경 파일:\n", req.PrjID)
	for path := range files {
		prBody += fmt.Sprintf("- %s\n", path)
	}

	msg := req.Message
	if msg == "" {
		msg = prTitle
	}

	publishResult, sc := callPlatform("POST", fmt.Sprintf("/DocRepo/%s/git/publish", req.DocRepoID),
		map[string]interface{}{
			"branch":  branchName,
			"files":   files,
			"message": msg,
			"prTitle": prTitle,
			"prBody":  prBody,
		})
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

func callPlatform(method, path string, body interface{}) (interface{}, int) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, platformAPI+path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 500
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var result interface{}
	json.Unmarshal(data, &result)
	return result, resp.StatusCode
}
