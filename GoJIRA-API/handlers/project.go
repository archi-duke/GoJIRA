package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

// archSettingsAllowedKeys — PATCH /Project/{jiraCode}/archSettings 가 허용하는 서브필드.
// 이 집합에 들어있는 키만 bson 의 `archSettings.<key>` 경로로 $set 된다.
var archSettingsAllowedKeys = map[string]struct{}{
	"normalizedAttributes": {},
	"priorityWeight":       {},
	"chosenStyles":         {},
	"satamDone":            {},
	"designIntegrated":     {},
	"viewsIncluded":        {},
	// 결정적 조립용 보조 데이터 (이슈에 없는 메타)
	"styleQAWeights":       {}, // style-define2 후보 비교 가중치
	// drift-check.py — workflow stage hash baseline & deferred impact tracking
	"lastRunHash":      {},
	"lastRunAt":        {},
	"deferredImpacts":  {},
}

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

// PATCH /api/v{version}/Project/{jiraCode}/archSettings
//
// Body: { normalizedAttributes?, priorityWeight?, chosenStyles?, satamDone?,
//         designIntegrated?, viewsIncluded? }   (모두 optional, 포함된 것만 merge)
//
// whitelist 밖의 키가 있으면 400. 하나도 없으면 400. 프로젝트가 없으면 404.
// 성공 시 { jiraCode, archSettings, updated:[keys] } 반환.
func ProjectArchSettingsPatchHandler(w http.ResponseWriter, r *http.Request) {
	jiraCode := chi.URLParam(r, "jiraCode")
	if jiraCode == "" {
		respondError(w, http.StatusBadRequest, "jiraCode path param required")
		return
	}

	var body map[string]interface{}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	fields := bson.M{}
	updatedKeys := make([]string, 0, len(body))
	var rejected []string
	for k, v := range body {
		if _, ok := archSettingsAllowedKeys[k]; !ok {
			rejected = append(rejected, k)
			continue
		}
		fields["archSettings."+k] = v
		updatedKeys = append(updatedKeys, k)
	}
	if len(rejected) > 0 {
		allowed := make([]string, 0, len(archSettingsAllowedKeys))
		for k := range archSettingsAllowedKeys {
			allowed = append(allowed, k)
		}
		respondError(w, http.StatusBadRequest,
			fmt.Sprintf("unknown keys: %v (allowed: %v)", rejected, allowed))
		return
	}
	if len(fields) == 0 {
		respondError(w, http.StatusBadRequest, "empty body — at least one whitelisted key required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	matched, err := db.UpdateProjectFields(ctx, jiraCode, fields)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !matched {
		respondError(w, http.StatusNotFound, fmt.Sprintf("project %q not found", jiraCode))
		return
	}

	prj := db.FindProjectByCode(ctx, jiraCode)
	var arch interface{}
	if prj != nil {
		arch = bsonGetDoc(prj, "archSettings")
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"jiraCode":     jiraCode,
		"archSettings": arch,
		"updated":      updatedKeys,
	})
}

// POST /api/v{version}/Project/{jiraCode}/reset
//
// Body: { confirm: bool, resetIssues?: bool, resetArchSettings?: bool }
//   - confirm=false (기본): dry-run — 영향 범위만 반환
//   - confirm=true: 실제 실행
//   - resetIssues 기본 true, resetArchSettings 기본 true
//
// 처리:
//   - resetIssues: projectCode 로 시작하는 모든 이슈를 deleteIssueInternal 로 삭제
//     (Gitea YAML + Mongo issues 컬렉션 + 모든 프로젝트의 issues 배열에서 제거)
//     + project.issues 배열 자체를 [] 로 초기화 (안전망)
//   - resetArchSettings: project.archSettings 필드 $unset
//
// 응답: { jiraCode, mode, deleted, failed, archSettingsCleared, issueCount, ... }
//
// 문서(business/*, architecture/*) 초기화는 이 엔드포인트가 담당하지 않음 — Platform-API
// /DocRepo/{code}/file 을 호출자(reset-project 스킬)가 반복 호출.
func ProjectResetHandler(w http.ResponseWriter, r *http.Request) {
	jiraCode := chi.URLParam(r, "jiraCode")
	if jiraCode == "" {
		respondError(w, http.StatusBadRequest, "jiraCode path param required")
		return
	}

	var body struct {
		Confirm           bool  `json:"confirm"`
		ResetIssues       *bool `json:"resetIssues"`
		ResetArchSettings *bool `json:"resetArchSettings"`
		DeleteProject     bool  `json:"deleteProject"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	user, ok := resolveUser(r, "")
	if !ok {
		respondError(w, http.StatusForbidden, "user required (X-User-Id)")
		return
	}

	resetIssues := true
	if body.ResetIssues != nil {
		resetIssues = *body.ResetIssues
	}
	resetArch := true
	if body.ResetArchSettings != nil {
		resetArch = *body.ResetArchSettings
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	prj := db.FindProjectByCode(ctx, jiraCode)
	if prj == nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("project %q not found", jiraCode))
		return
	}

	// 영향 범위 조회: 이 프로젝트(projectId) 에 속한 이슈들
	issueKeys := []string{}
	if resetIssues {
		issues, err := db.FindAllIssues(ctx, bson.M{"projectId": prj["_id"]})
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, iss := range issues {
			if k, _ := iss["key"].(string); k != "" {
				issueKeys = append(issueKeys, k)
			}
		}
	}

	// dry-run
	if !body.Confirm {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"jiraCode": jiraCode,
			"mode":     "dry-run",
			"wouldDelete": map[string]interface{}{
				"issueCount":          len(issueKeys),
				"issueKeys":           issueKeys,
				"resetArchSettings":   resetArch,
				"archSettingsPresent": bsonGetDoc(prj, "archSettings") != nil,
				"deleteProject":       body.DeleteProject,
			},
			"message": fmt.Sprintf("DRY-RUN: %d이슈 삭제 예정. 실제 실행은 confirm=true 로 재호출.", len(issueKeys)),
		})
		return
	}

	// 실행
	deleted := make([]string, 0, len(issueKeys))
	failed := make([]map[string]interface{}, 0)
	for _, k := range issueKeys {
		if err := deleteIssueInternal(ctx, jiraCode, k, user); err != nil {
			failed = append(failed, map[string]interface{}{"key": k, "error": err.Error()})
			continue
		}
		deleted = append(deleted, k)
	}

	archCleared := false
	if resetArch {
		if err := db.UnsetProjectField(ctx, jiraCode, "archSettings"); err != nil {
			log.Printf("archSettings unset failed: %v", err)
		} else {
			archCleared = true
		}
	}

	// usecase_artifacts 도 함께 정리 — relink/강제 재생성 모드의 stale 데이터 재활용 방지.
	if err := db.DeleteUsecaseArtifact(ctx, prj["_id"]); err != nil {
		log.Printf("usecase_artifacts delete failed: %v", err)
	}

	projectDeleted := false
	if body.DeleteProject {
		if ok, err := db.DeleteProjectByCode(ctx, jiraCode); err != nil {
			log.Printf("project delete failed: %v", err)
		} else {
			projectDeleted = ok
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"jiraCode":            jiraCode,
		"mode":                "executed",
		"deleted":             deleted,
		"deletedCount":        len(deleted),
		"failed":              failed,
		"failedCount":         len(failed),
		"archSettingsCleared": archCleared,
		"projectDeleted":      projectDeleted,
	})
}

// GET /api/v{version}/Project/{jiraCode}/dump
//
// archive-project 스킬의 백업용 read-only 엔드포인트.
// 응답: { project: {...}, issues: [...] }
//   - project: projects 컬렉션 해당 문서 전체
//   - issues: issues 컬렉션에서 key prefix 매칭된 이슈들
//
// 프로젝트가 없으면 404.
func ProjectDumpHandler(w http.ResponseWriter, r *http.Request) {
	jiraCode := chi.URLParam(r, "jiraCode")
	if jiraCode == "" {
		respondError(w, http.StatusBadRequest, "jiraCode path param required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	prj := db.FindProjectByCode(ctx, jiraCode)
	if prj == nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("project %q not found", jiraCode))
		return
	}

	issuesArr, err := db.FindAllIssues(ctx, bson.M{"projectId": prj["_id"]})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if issuesArr == nil {
		issuesArr = []bson.M{}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"project": prj,
		"issues":  issuesArr,
	})
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
