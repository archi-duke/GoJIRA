package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/jiraclient"
	"gojira-api/models"
)

// POST /api/v{version}/Issue
func IssueCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Summary == "" {
		respondError(w, http.StatusBadRequest, "summary is required")
		return
	}

	prjKey := req.ProjectKey
	if prjKey == "" {
		prjKey = req.PrjID
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// JIRA에 이슈 생성
	fields := map[string]interface{}{
		"summary":   req.Summary,
		"issuetype": map[string]string{"name": req.IssueType},
	}
	if req.Desc != "" {
		fields["description"] = req.Desc
	}
	if len(req.Labels) > 0 {
		fields["labels"] = req.Labels
	}
	if req.ReqType != "" {
		fields["customfield_12555"] = map[string]string{"value": req.ReqType}
	}
	if req.SubKey != "" {
		fields["customfield_15209"] = req.SubKey
	}
	if prjKey != "" {
		fields["project"] = map[string]string{"key": prjKey}
	}

	result, err := jiraclient.CreateIssue(fields)
	if err != nil {
		respondError(w, http.StatusBadGateway, "JIRA 이슈 생성 실패: "+err.Error())
		return
	}

	jiraKey, _ := result["key"].(string)
	if jiraKey == "" {
		respondError(w, http.StatusBadGateway, "JIRA에서 키를 받지 못했습니다")
		return
	}

	// JIRA에서 전체 이슈 정보 가져와서 MongoDB에 저장
	doc := bson.M{
		"key":       jiraKey,
		"issueType": req.IssueType,
		"subkey":    req.SubKey,
		"summary":   req.Summary,
		"labels":    req.Labels,
		"reqtype":   req.ReqType,
		"_dirty":    false,
		"_syncedAt": time.Now(),
	}
	if req.Desc != "" {
		doc["description"] = req.Desc
	}

	jiraIssue, err := jiraclient.GetIssue(jiraKey)
	if err == nil {
		doc = db.TransformToCache(jiraIssue)
		doc["_dirty"] = false
		doc["_syncedAt"] = time.Now()
	}

	db.UpsertIssue(ctx, jiraKey, doc)

	// 프로젝트 issues 배열에 키 추가
	db.AddIssueToProject(ctx, prjKey, jiraKey)

	log.Printf("created JIRA issue %s (subkey: %s)", jiraKey, req.SubKey)
	respondJSON(w, http.StatusOK, map[string]string{"key": jiraKey, "subkey": req.SubKey})
}

// GET /api/v{version}/Issue — read from MongoDB
func IssueGetHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueGetRequest
	if err := parseXBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid XBody header")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var filter bson.M
	switch req.InfoType {
	case "designs":
		filter = bson.M{"issueType": bson.M{
			"$in": []string{"Architectueral Design", "Detaied Design"},
		}}
	case "nonTestIssues":
		filter = bson.M{"issueType": bson.M{"$ne": "Test"}}
	case "isBusy":
		respondJSON(w, http.StatusOK, false)
		return
	default:
		filter = nil
	}

	// Filter by project: lookup issues array from projects collection
	if req.PrjID != "" {
		prjDoc := db.FindProjectByCode(ctx, req.PrjID)
		if prjDoc != nil {
			var issueKeys []string
			if raw, ok := prjDoc["issues"]; ok {
				switch arr := raw.(type) {
				case bson.A:
					for _, v := range arr {
						if s, ok := v.(string); ok {
							issueKeys = append(issueKeys, s)
						}
					}
				case []interface{}:
					for _, v := range arr {
						if s, ok := v.(string); ok {
							issueKeys = append(issueKeys, s)
						}
					}
				}
			}
			if len(issueKeys) > 0 {
				prjFilter := bson.M{"key": bson.M{"$in": issueKeys}}
				if filter != nil {
					filter = bson.M{"$and": []bson.M{filter, prjFilter}}
				} else {
					filter = prjFilter
				}
			} else {
				respondJSON(w, http.StatusOK, []bson.M{})
				return
			}
		}
	}

	issues, err := db.FindAllIssues(ctx, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Strip internal fields from response
	cleaned := make([]bson.M, 0, len(issues))
	for _, issue := range issues {
		delete(issue, "_id")
		delete(issue, "_dirty")
		delete(issue, "_syncedAt")
		cleaned = append(cleaned, issue)
	}

	respondJSON(w, http.StatusOK, cleaned)
}

// PUT /api/v{version}/Issue — update in MongoDB, mark dirty
func IssueUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Build field updates (flat cache schema — no "fields." prefix)
	updates := bson.M{}
	for k, v := range req.Data {
		updates[k] = v
	}
	updates["_dirty"] = true

	if err := db.UpdateIssueFields(ctx, req.Key, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	db.SyncIssueToJira(req.Key)
	respondJSON(w, http.StatusOK, "issue updated")
}

// DELETE /api/v{version}/Issue — delete from MongoDB and JIRA
func IssueDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 이슈 key의 prefix와 요청 프로젝트 코드 비교
	isOwnIssue := req.PrjID != "" && strings.HasPrefix(req.Key, req.PrjID+"-")

	if isOwnIssue {
		// 자기 과제 이슈: DB에서 삭제 + 모든 프로젝트 issues 배열에서 제거
		if err := db.DeleteIssueByKey(ctx, req.Key); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		projects, _ := db.FindAllProjects(ctx, nil)
		for _, prj := range projects {
			code := extractJiraCode(prj)
			if code != "" {
				db.RemoveIssueFromProject(ctx, code, req.Key)
			}
		}
		// Async delete from JIRA
		go func() {
			if err := jiraclient.DeleteIssue(req.Key); err != nil {
				log.Printf("async JIRA delete %s failed: %v", req.Key, err)
			}
		}()
	} else {
		// 타 과제 이슈: 프로젝트 목록에서만 제거 (DB/JIRA 삭제 안 함)
		if req.PrjID != "" {
			db.RemoveIssueFromProject(ctx, req.PrjID, req.Key)
		}
	}

	respondJSON(w, http.StatusOK, "issue deleted")
}
