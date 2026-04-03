package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

// POST /api/v{version}/Project
func ProjectCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.ProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	code := req.MainJiraCode()
	if req.Name == "" || code == "" {
		respondError(w, http.StatusBadRequest, "name and jiraCodes.main.jiraCode are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	doc := buildProjectDoc(req)
	doc["createdAt"] = time.Now()

	if err := db.UpsertProject(ctx, code, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 설계문서 저장소 자동 생성
	ensureDocRepo(code, req.Name)

	projects, _ := db.FindAllProjects(ctx, nil)
	respondJSON(w, http.StatusOK, projects)
}

// GET /api/v{version}/Project
func ProjectGetHandler(w http.ResponseWriter, r *http.Request) {
	var req models.ProjectGetRequest
	parseXBody(r, &req)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var filter bson.M
	if req.JiraCode != "" {
		filter = bson.M{"jiraCodes.main.jiraCode": req.JiraCode}
	}

	projects, err := db.FindAllProjects(ctx, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, projects)
}

// PUT /api/v{version}/Project
func ProjectUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.ProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	code := req.MainJiraCode()
	if code == "" {
		respondError(w, http.StatusBadRequest, "jiraCodes.main.jiraCode is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	doc := buildProjectDoc(req)

	if err := db.UpsertProject(ctx, code, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 설계문서 저장소 자동 생성 (없으면)
	ensureDocRepo(code, req.Name)

	// auditors → docRepo에도 동기화
	if req.Auditors != nil {
		syncAuditorsToDocRepo(code, req.Auditors)
	}

	projects, _ := db.FindAllProjects(ctx, nil)
	respondJSON(w, http.StatusOK, projects)
}

func buildProjectDoc(req models.ProjectRequest) bson.M {
	code := req.MainJiraCode()
	doc := bson.M{
		"name":      req.Name,
		"jiraCodes": bson.M{"main": bson.M{"jiraCode": code}},
		"status":    req.Status,
		"devTeam":   req.DevTeam,
		"devGroup":  req.DevGroup,
		"feature":   req.Feature,
		"architect": req.Architect,
		"PL":        req.PL,
	}
	// nil인 필드는 $set에서 제외하여 기존 값 보존
	if req.Members != nil {
		doc["members"] = req.Members
	}
	if req.Schedules != nil {
		doc["schedules"] = req.Schedules
	}
	doc["issueQuery"] = req.IssueQuery
	if req.PLMCode != nil {
		doc["PLMCode"] = req.PLMCode
	}
	if req.Diagrams != nil {
		doc["diagrams"] = req.Diagrams
	}
	if req.Documents != nil {
		doc["documents"] = req.Documents
	}
	if req.Auditors != nil {
		doc["auditors"] = req.Auditors
	}
	return doc
}

func nilToEmpty(v interface{}) interface{} {
	if v == nil {
		return []interface{}{}
	}
	return v
}

func syncAuditorsToDocRepo(jiraCode string, auditors []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prj := db.FindProjectByCode(ctx, jiraCode)
	if prj == nil {
		return
	}
	repoID := getDocRepoID(prj)
	if repoID == "" {
		return
	}
	callPlatform("PUT", fmt.Sprintf("/DocRepo/%s", repoID), map[string]interface{}{"auditors": auditors})
	log.Printf("syncAuditors: %s → docRepo %s auditors=%v", jiraCode, repoID, auditors)
}

func getDocRepoID(prj bson.M) string {
	log.Printf("getDocRepoID: documents type=%T value=%v", prj["documents"], prj["documents"])
	switch docs := prj["documents"].(type) {
	case bson.M:
		if id, ok := docs["repo"].(string); ok {
			return id
		}
	case bson.D:
		for _, e := range docs {
			if e.Key == "repo" {
				if s, ok := e.Value.(string); ok {
					return s
				}
			}
		}
	case map[string]interface{}:
		if id, ok := docs["repo"].(string); ok {
			return id
		}
	}
	return ""
}

// ensureDocRepo 프로젝트에 docRepoId가 없으면 자동 생성
func ensureDocRepo(jiraCode, projectName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prj := db.FindProjectByCode(ctx, jiraCode)
	if prj == nil {
		return
	}

	// documents.repo 확인 — 이미 있으면 skip
	repoID := getDocRepoID(prj)
	if repoID != "" {
		log.Printf("ensureDocRepo: %s already has documents.repo=%s", jiraCode, repoID)
		return
	}
	log.Printf("ensureDocRepo: creating repo for %s (%s)", jiraCode, projectName)

	repoName := fmt.Sprintf("%s_%s", jiraCode, time.Now().Format("20060102150405"))
	description := fmt.Sprintf("%s의 설계 문서", projectName)

	result, sc := callPlatform("POST", "/DocRepo", map[string]interface{}{
		"repoName":    repoName,
		"projectId":   jiraCode,
		"description": description,
		"createdBy":   "system",
		"auditors":    []string{},
	})
	log.Printf("ensureDocRepo: Platform API response status=%d, result=%v", sc, result)
	if sc >= 400 {
		log.Printf("ensureDocRepo: repo 생성 실패 for %s (status=%d)", jiraCode, sc)
		return
	}

	// docRepoId + repoName 추출 및 프로젝트에 연결
	if m, ok := result.(map[string]interface{}); ok {
		if id, ok := m["_id"]; ok {
			docRepoID := fmt.Sprintf("%v", id)
			db.UpdateProjectField(ctx, jiraCode, "documents.repo", docRepoID)
			db.UpdateProjectField(ctx, jiraCode, "documents.repoName", repoName)
			log.Printf("ensureDocRepo: %s → repo %s (id=%s)", jiraCode, repoName, docRepoID)
		}
	}
}
