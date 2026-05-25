package handlers

// 트랙 E — 이슈 로컬 모델 ingest (local-agent-issue-model.md).
//
// 로컬 clone 의 data/issues/*.yaml (= 진실) 을 Mongo 캐시로 반영한다.
//   - 단건  POST   /Issue/ingest            : 로컬 편집 dual-write (②).
//   - 단건  DELETE /Issue/ingest?key=        : 로컬 YAML 삭제 반영 (git history 가 휴지통).
//   - 벌크  POST   /Project/{jiraCode}/issues/ingest : 과제 열기 콜드스타트 + reconcile.
//
// 어느 경우든 GoJIRA-API 는 fs 에 손대지 않는다 — YAML 본문을 받아 파싱해 Mongo 만 갱신한다.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"

	"gojira-api/db"
)

// parseIssueYAML — YAML 본문 → doc + key 추출.
func parseIssueYAML(yamlContent string) (bson.M, string, error) {
	var doc bson.M
	if err := yaml.Unmarshal([]byte(yamlContent), &doc); err != nil {
		return nil, "", fmt.Errorf("yaml parse: %w", err)
	}
	key, _ := doc["key"].(string)
	if key == "" {
		return nil, "", fmt.Errorf("yaml missing 'key' field")
	}
	return doc, key, nil
}

// ingestOneYAML — YAML 1건 파싱 → projectID 해석 → Mongo 캐시 upsert. 반영된 key 반환.
func ingestOneYAML(ctx context.Context, yamlContent string) (string, error) {
	doc, key, err := parseIssueYAML(yamlContent)
	if err != nil {
		return "", err
	}
	code := keyToProjectCode(key)
	if code == "" {
		return "", fmt.Errorf("cannot derive project code from key %s", key)
	}
	prj := db.FindProjectByCode(ctx, code)
	if prj == nil {
		return "", fmt.Errorf("project not found: %s", code)
	}
	if err := upsertIssueCacheFromDoc(ctx, key, prj["_id"], doc); err != nil {
		return "", err
	}
	return key, nil
}

// POST /Issue/ingest — 단건 YAML → Mongo 캐시 upsert (로컬 편집 dual-write).
// body: { "yaml": "<yaml content>" }
func IssueIngestHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := resolveUser(r, ""); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	var req struct {
		YAML string `json:"yaml"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.YAML == "" {
		respondError(w, http.StatusBadRequest, "yaml is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	key, err := ingestOneYAML(ctx, req.YAML)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"key": key, "status": "ingested"})
}

// DELETE /Issue/ingest?key= — 로컬 YAML 삭제 반영: Mongo 캐시에서 제거.
// git history 가 휴지통 역할을 하므로 캐시는 hard delete.
func IssueIngestDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := resolveUser(r, ""); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "key query param is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := db.DeleteIssueByKey(ctx, key); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"key": key, "status": "deleted"})
}

// POST /Project/{jiraCode}/issues/ingest — 벌크 ingest (콜드스타트).
// body: { "files": ["<yaml1>", "<yaml2>", ...], "reconcile": true }
// reconcile=true 면 ingest 집합에 없는 "이 과제 소속(key prefix=code)" 캐시 이슈를 제거(YAML=진실).
// 링크만 걸린 타 과제 이슈는 건드리지 않는다.
func ProjectIssuesIngestHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "jiraCode")
	if _, ok := resolveUser(r, ""); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	var req struct {
		Files     []string `json:"files"`
		Reconcile bool     `json:"reconcile"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	prj := db.FindProjectByCode(ctx, code)
	if prj == nil {
		respondError(w, http.StatusNotFound, "project not found: "+code)
		return
	}
	projectID := prj["_id"]

	ingested := map[string]bool{}
	failed := []map[string]string{}
	for _, y := range req.Files {
		doc, key, err := parseIssueYAML(y)
		if err != nil {
			failed = append(failed, map[string]string{"error": err.Error()})
			continue
		}
		if err := upsertIssueCacheFromDoc(ctx, key, projectID, doc); err != nil {
			failed = append(failed, map[string]string{"key": key, "error": err.Error()})
			continue
		}
		ingested[key] = true
	}

	removed := 0
	if req.Reconcile {
		existing, _ := db.FindAllIssues(ctx, bson.M{"projectId": projectID})
		prefix := code + "-"
		for _, doc := range existing {
			k, _ := doc["key"].(string)
			// 이 과제 소속(key prefix=code)이면서 ingest 집합에 없으면 캐시에서 제거.
			if k != "" && strings.HasPrefix(k, prefix) && !ingested[k] {
				if err := db.DeleteIssueByKey(ctx, k); err == nil {
					removed++
				}
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ingested": len(ingested),
		"removed":  removed,
		"failed":   failed,
		"syncedAt": time.Now(),
	})
}
