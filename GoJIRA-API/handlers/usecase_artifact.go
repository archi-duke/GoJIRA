package handlers

// usecase_artifacts — usecase-analyze2 가 결정적 조립을 위해 영속화하는 보조 데이터.
//
//   GET /api/v{ver}/Project/{jiraCode}/usecase-artifacts
//   PUT /api/v{ver}/Project/{jiraCode}/usecase-artifacts
//
// 저장 필드: domain / userStories / useCases / diagrams / vosKeys (선택).
// Feature 6필드는 issues 컬렉션에서 직접 읽으므로 여기 저장하지 않음.

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"gojira-api/db"
)

var usecaseArtifactAllowedKeys = map[string]struct{}{
	"domain":       {},
	"userStories":  {},
	"useCases":     {},
	"diagrams":     {},
	"vosKeys":      {},
}

// GET /Project/{jiraCode}/usecase-artifacts
//
// 프로젝트 없음: 404
// artifact 없음: 200 + {projectCode, exists:false} (클라이언트 분기용)
// artifact 있음: 200 + {projectCode, exists:true, domain, userStories, useCases, diagrams, vosKeys, schemaVersion, updatedAt, updatedBy}
func UsecaseArtifactGetHandler(w http.ResponseWriter, r *http.Request) {
	jiraCode := chi.URLParam(r, "jiraCode")
	if jiraCode == "" {
		respondError(w, http.StatusBadRequest, "jiraCode path param required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	prj := db.FindProjectByCode(ctx, jiraCode)
	if prj == nil {
		respondError(w, http.StatusNotFound, "project not found: "+jiraCode)
		return
	}
	projectID := prj["_id"]

	doc, err := db.FindUsecaseArtifact(ctx, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if doc == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"projectCode": jiraCode,
			"exists":      false,
		})
		return
	}

	out := map[string]interface{}{
		"projectCode":   jiraCode,
		"exists":        true,
		"schemaVersion": doc["schemaVersion"],
		"updatedAt":     doc["updatedAt"],
		"updatedBy":     doc["updatedBy"],
	}
	for k := range usecaseArtifactAllowedKeys {
		if v, ok := doc[k]; ok {
			out[k] = v
		}
	}
	respondJSON(w, http.StatusOK, out)
}

// PUT /Project/{jiraCode}/usecase-artifacts
//
// Body: { domain?, userStories?, useCases?, diagrams?, vosKeys? } — 포함된 키만 partial $set
// whitelist 밖의 키는 400. 모두 비어있으면 400.
func UsecaseArtifactPutHandler(w http.ResponseWriter, r *http.Request) {
	jiraCode := chi.URLParam(r, "jiraCode")
	if jiraCode == "" {
		respondError(w, http.StatusBadRequest, "jiraCode path param required")
		return
	}
	user, ok := resolveUser(r, "")
	if !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}

	var body map[string]interface{}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	fields := map[string]interface{}{}
	rejected := []string{}
	for k, v := range body {
		if _, ok := usecaseArtifactAllowedKeys[k]; !ok {
			rejected = append(rejected, k)
			continue
		}
		fields[k] = v
	}
	if len(rejected) > 0 {
		respondError(w, http.StatusBadRequest, "unknown keys: "+joinStrings(rejected))
		return
	}
	if len(fields) == 0 {
		respondError(w, http.StatusBadRequest, "empty body — at least one of domain/userStories/useCases/diagrams/vosKeys required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	prj := db.FindProjectByCode(ctx, jiraCode)
	if prj == nil {
		respondError(w, http.StatusNotFound, "project not found: "+jiraCode)
		return
	}
	projectID := prj["_id"]

	if err := db.UpsertUsecaseArtifactPartial(ctx, projectID, fields, user); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated := make([]string, 0, len(fields))
	for k := range fields {
		updated = append(updated, k)
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"projectCode": jiraCode,
		"updated":     updated,
	})
}

func joinStrings(xs []string) string {
	out := ""
	for i, s := range xs {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
