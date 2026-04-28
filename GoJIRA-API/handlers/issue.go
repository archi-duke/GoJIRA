package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"

	"gojira-api/clients"
	"gojira-api/db"
	"gojira-api/models"
)

// data/issues/<subkey>.yaml 경로 (Python issuestore._yaml_path 와 동일 규칙).
var _subkeySafe = regexp.MustCompile(`[^A-Za-z0-9_\-]+`)

func issueYamlPath(subkey string) string {
	safe := _subkeySafe.ReplaceAllString(subkey, "_")
	return "data/issues/" + safe + ".yaml"
}

// POST /api/v{version}/Issue — 이슈 생성.
//
// 동작 변경 (YAML 진실 원천):
//  1. key / subkey 자체 할당 (JIRA 호출 제거)
//  2. YAML 직렬화 → docs/{user} 브랜치 커밋
//  3. Mongo 캐시 upsert + project.issues 배열 갱신
//
// JIRA 푸시는 별도 기능(/Issue/push-jira, 추후)에서 처리.
func IssueCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, ok := resolveUser(r, req.User)
	if !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	if req.Summary == "" {
		respondError(w, http.StatusBadRequest, "summary is required")
		return
	}
	if req.IssueType == "" {
		respondError(w, http.StatusBadRequest, "issueType is required")
		return
	}
	prjKey := req.ProjectKey
	if prjKey == "" {
		prjKey = req.PrjID
	}
	if prjKey == "" {
		respondError(w, http.StatusBadRequest, "prjID/projectKey required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// 0) project 존재 확인 + _id 확보 (이슈는 project._id 로 단일 소속)
	prj := db.FindProjectByCode(ctx, prjKey)
	if prj == nil {
		respondError(w, http.StatusNotFound, "project not found: "+prjKey)
		return
	}
	projectID := prj["_id"]

	// 1) key 할당
	num, err := db.NextIssueNum(ctx, prjKey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "key 할당 실패: "+err.Error())
		return
	}
	key := fmt.Sprintf("%s-%d", prjKey, num)

	// 2) subkey 할당 (미전달 시 issueType 매핑)
	subkey := req.SubKey
	if subkey == "" {
		prefix := db.SubkeyPrefix(req.IssueType, req.ReqType)
		if prefix == "" {
			respondError(w, http.StatusBadRequest, "subkey required for issueType "+req.IssueType)
			return
		}
		subkey, err = db.NextSubkey(ctx, prjKey, prefix)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "subkey 할당 실패: "+err.Error())
			return
		}
	}

	// 3) YAML doc build
	now := time.Now().Format("2006-01-02T15:04:05.000+0900")
	doc := map[string]interface{}{
		"key":         key,
		"subkey":      subkey,
		"issueType":   req.IssueType,
		"summary":     req.Summary,
		"description": req.Desc,
		"status":      "To Do",
		"priority":    "Medium",
		"labels":      req.Labels,
		"created":     now,
		"updated":     now,
	}
	if req.ReqType != "" && req.ReqType != "null" {
		doc["reqtype"] = req.ReqType
	}

	yamlBytes, err := yaml.Marshal(doc)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "yaml marshal: "+err.Error())
		return
	}

	// 4) YAML 커밋 (user 브랜치)
	msg := fmt.Sprintf("[issue] %s %s", key, truncate(req.Summary, 40))
	if err := clients.SaveFile(prjKey, issueYamlPath(subkey), string(yamlBytes), msg, user); err != nil {
		respondError(w, http.StatusBadGateway, "YAML 저장 실패: "+err.Error())
		return
	}

	// 5) Mongo 캐시 upsert (실패해도 YAML 이 진실이므로 경고만)
	cacheDoc := bson.M{}
	for k, v := range doc {
		cacheDoc[k] = v
	}
	cacheDoc["projectId"] = projectID
	cacheDoc["_dirty"] = false
	cacheDoc["_syncedAt"] = time.Now()
	if err := db.UpsertIssue(ctx, key, cacheDoc); err != nil {
		log.Printf("cache upsert warn key=%s: %v", key, err)
	}

	log.Printf("created issue %s subkey=%s user=%s (YAML)", key, subkey, user)
	respondJSON(w, http.StatusOK, map[string]string{"key": key, "subkey": subkey})
}

// GET /api/v{version}/Issue — Mongo 캐시 read.
func IssueGetHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueGetRequest
	if err := parseXBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid XBody header")
		return
	}
	if _, ok := resolveUser(r, req.User); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var filter bson.M
	switch req.InfoType {
	case "designs":
		filter = bson.M{"issueType": bson.M{
			"$in": []string{"Architectural Design", "Detailed Design"},
		}}
	case "nonTestIssues":
		filter = bson.M{"issueType": bson.M{"$ne": "Test"}}
	case "isBusy":
		respondJSON(w, http.StatusOK, false)
		return
	default:
		filter = nil
	}

	if req.PrjID != "" {
		prjDoc := db.FindProjectByCode(ctx, req.PrjID)
		if prjDoc == nil {
			respondJSON(w, http.StatusOK, []bson.M{})
			return
		}
		// 이슈는 projectId 로 단일 소속
		prjFilter := bson.M{"projectId": prjDoc["_id"]}
		if filter != nil {
			filter = bson.M{"$and": []bson.M{filter, prjFilter}}
		} else {
			filter = prjFilter
		}
	}

	issues, err := db.FindAllIssues(ctx, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	cleaned := make([]bson.M, 0, len(issues))
	for _, issue := range issues {
		delete(issue, "_id")
		delete(issue, "_dirty")
		delete(issue, "_syncedAt")
		cleaned = append(cleaned, issue)
	}

	respondJSON(w, http.StatusOK, cleaned)
}

// PUT /api/v{version}/Issue — 필드 업데이트.
//
// 동작: Mongo 캐시 문서 로드 → Data 병합 → YAML 재생성 → user 브랜치 커밋 → 캐시 업데이트.
// JIRA 동기화는 제거. 추후 별도 push 플로우에서 처리.
func IssueUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, ok := resolveUser(r, req.User)
	if !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	existing, err := db.FindIssueByKey(ctx, req.Key)
	if err != nil || existing == nil {
		respondError(w, http.StatusNotFound, "issue not found: "+req.Key)
		return
	}

	subkey, _ := existing["subkey"].(string)
	if subkey == "" {
		respondError(w, http.StatusConflict, "issue has no subkey — cannot locate YAML")
		return
	}
	code := keyToProjectCode(req.Key)
	if code == "" {
		respondError(w, http.StatusBadRequest, "cannot derive project code from key")
		return
	}

	// 병합 — 내부 필드 제외
	merged := bson.M{}
	for k, v := range existing {
		if k == "_id" || k == "_dirty" || k == "_syncedAt" {
			continue
		}
		merged[k] = v
	}
	for k, v := range req.Data {
		merged[k] = v
	}
	merged["updated"] = time.Now().Format("2006-01-02T15:04:05.000+0900")

	yamlBytes, err := yaml.Marshal(merged)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "yaml marshal: "+err.Error())
		return
	}

	msg := fmt.Sprintf("[issue] update %s", req.Key)
	if err := clients.SaveFile(code, issueYamlPath(subkey), string(yamlBytes), msg, user); err != nil {
		respondError(w, http.StatusBadGateway, "YAML 저장 실패: "+err.Error())
		return
	}

	merged["_dirty"] = false
	merged["_syncedAt"] = time.Now()
	if err := db.UpsertIssue(ctx, req.Key, merged); err != nil {
		log.Printf("cache upsert warn key=%s: %v", req.Key, err)
	}

	respondJSON(w, http.StatusOK, "issue updated")
}

// DELETE /api/v{version}/Issue — 이슈 삭제.
//
// 자기 과제 이슈: YAML 삭제 → 캐시 삭제 → 모든 프로젝트 issues 배열에서 제거.
// 타 과제 이슈(링크만 걸려있는 경우): 현재 프로젝트 issues 배열에서만 제거.
// JIRA 삭제 호출은 제거.
func IssueDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, ok := resolveUser(r, req.User)
	if !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := deleteIssueInternal(ctx, req.PrjID, req.Key, user); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, "issue deleted")
}

// deleteIssueInternal encapsulates the full delete flow: Gitea YAML + Mongo issues
// collection. Called by IssueDeleteHandler (single key) and ProjectResetHandler (bulk).
// Gitea YAML delete failure is logged but not fatal — callers still succeed on Mongo-level delete.
// 이슈는 projectId 로 단일 소속이므로 별도 배열 정리는 불필요.
func deleteIssueInternal(ctx context.Context, code, key, user string) error {
	isOwnIssue := code != "" && strings.HasPrefix(key, code+"-")

	if isOwnIssue {
		existing, _ := db.FindIssueByKey(ctx, key)
		subkey, _ := existing["subkey"].(string)
		if subkey != "" {
			if err := clients.DeleteFile(code, issueYamlPath(subkey), fmt.Sprintf("[issue] delete %s", key), user); err != nil {
				log.Printf("YAML delete warn key=%s: %v", key, err)
			}
		}
	}
	if err := db.DeleteIssueByKey(ctx, key); err != nil {
		return err
	}
	return nil
}

// ── 유틸 ──

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func keyToProjectCode(key string) string {
	idx := strings.LastIndex(key, "-")
	if idx <= 0 {
		return ""
	}
	return key[:idx]
}
