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

	if req.Status == "Deleted" {
		// 프로젝트 삭제 시 DocRepo(Gitea + 로컬 클론) 정리
		go func() {
			_, sc := callPlatform("DELETE", fmt.Sprintf("/DocRepo/%s/github", code), nil)
			if sc >= 400 {
				log.Printf("deleteDocRepo: 실패 for %s (status=%d)", code, sc)
			} else {
				log.Printf("deleteDocRepo: %s repo 삭제 완료", code)
			}
		}()
	} else {
		// 설계문서 저장소 자동 생성 (없으면)
		ensureDocRepo(code, req.Name)
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
	// documents는 시스템이 관리 — 프론트에서 덮어쓰지 않음
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

func getDocRepoName(prj bson.M) string {
	return bsonGetStr(bsonGetDoc(prj, "documents"), "repoName")
}

// ensureDocRepo 프로젝트에 repo가 없으면 Platform-API로 자동 생성
func ensureDocRepo(jiraCode, projectName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prj := db.FindProjectByCode(ctx, jiraCode)
	if prj == nil {
		return
	}

	if getDocRepoName(prj) != "" {
		return
	}

	log.Printf("ensureDocRepo: creating repo for %s", jiraCode)
	_, sc := callPlatform("POST", fmt.Sprintf("/DocRepo/%s", jiraCode), nil)
	if sc >= 400 {
		log.Printf("ensureDocRepo: 실패 for %s (status=%d)", jiraCode, sc)
	}
	// 초기 파일은 Platform-API의 DocRepoCreateHandler에서 브랜치 보호 전에 생성됨
}
