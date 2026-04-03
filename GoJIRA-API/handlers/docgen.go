package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
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

	var issueKeys []interface{}
	if arr, ok := prjDoc["issues"]; ok {
		switch v := arr.(type) {
		case bson.A:
			issueKeys = []interface{}(v)
		case []interface{}:
			issueKeys = v
		}
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

	// 브랜치 생성
	branchName := fmt.Sprintf("docs/%s-%s", req.PrjID, time.Now().Format("20060102-150405"))
	_, statusCode := callPlatform("POST", fmt.Sprintf("/DocRepo/%s/branch", req.DocRepoID),
		map[string]string{"name": branchName, "from": "main"})
	if statusCode >= 400 {
		respondError(w, http.StatusBadGateway, "브랜치 생성 실패")
		return
	}

	// 파일 커밋
	for path, content := range files {
		b64 := base64.StdEncoding.EncodeToString([]byte(content))

		// 기존 파일 SHA 조회 (수정 시 필요)
		sha := ""
		fileData, sc := callPlatform("GET", fmt.Sprintf("/DocRepo/%s/file?path=%s&ref=%s", req.DocRepoID, path, branchName), nil)
		if sc == 200 {
			if m, ok := fileData.(map[string]interface{}); ok {
				sha, _ = m["sha"].(string)
			}
		}

		msg := req.Message
		if msg == "" {
			msg = "문서 갱신: " + path
		}

		_, sc = callPlatform("PUT", fmt.Sprintf("/DocRepo/%s/file", req.DocRepoID),
			map[string]interface{}{
				"path": path, "content": b64, "message": msg,
				"branch": branchName, "sha": sha,
			})
		if sc >= 400 {
			respondError(w, http.StatusBadGateway, fmt.Sprintf("파일 커밋 실패: %s", path))
			return
		}
	}

	// PR 생성
	prTitle := fmt.Sprintf("[%s] 설계문서 갱신 %s", req.PrjID, time.Now().Format("2006-01-02"))
	prBody := fmt.Sprintf("프로젝트 %s의 설계문서를 갱신합니다.\n\n변경 파일:\n", req.PrjID)
	for path := range files {
		prBody += fmt.Sprintf("- %s\n", path)
	}

	prResult, sc := callPlatform("POST", fmt.Sprintf("/DocRepo/%s/pr", req.DocRepoID),
		map[string]string{"title": prTitle, "body": prBody, "head": branchName, "base": "main"})
	if sc >= 400 {
		respondError(w, http.StatusBadGateway, "PR 생성 실패")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "설계문서 생성 및 PR 요청 완료",
		"branch":  branchName,
		"files":   len(files),
		"pr":      prResult,
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
