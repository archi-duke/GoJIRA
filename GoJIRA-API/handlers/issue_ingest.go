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
	"log"
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
	if err := validateIssueDoc(doc); err != nil {
		log.Printf("issue ingest: validation rejected key=%s: %v", key, err)
		return "", fmt.Errorf("validation failed: %w", err)
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
	// 실패 파일의 키 — reconcile 삭제 보호 대상. 검증/저장 실패는 "YAML 집합에 부재"가
	// 아니라 "상태 불명"이므로 캐시에서 지우면 안 된다 (2026-07-07 NBBALL resync 사고:
	// main 의 오염 YAML 49건이 검증 거부 → reconcile 이 정상 캐시 49건을 삭제).
	failedKeys := map[string]bool{}
	unparseable := 0 // 키조차 모르는 실패 — 어느 이슈인지 알 수 없음.
	for _, y := range req.Files {
		doc, key, err := parseIssueYAML(y)
		if err != nil {
			failed = append(failed, map[string]string{"error": err.Error()})
			unparseable++
			continue
		}
		if err := validateIssueDoc(doc); err != nil {
			log.Printf("issue ingest: validation rejected key=%s: %v", key, err)
			failed = append(failed, map[string]string{"key": key, "error": err.Error()})
			failedKeys[key] = true
			continue
		}
		if err := upsertIssueCacheFromDoc(ctx, key, projectID, doc); err != nil {
			failed = append(failed, map[string]string{"key": key, "error": err.Error()})
			failedKeys[key] = true
			continue
		}
		ingested[key] = true
	}

	removed := 0
	reconcileSkipped := false
	if req.Reconcile {
		if unparseable > 0 {
			// 키를 모르는 실패가 있으면 어떤 캐시 이슈가 그 파일이었는지 판별 불가 —
			// 부분 집합으로 삭제하면 정상 이슈가 지워질 수 있으므로 제거 단계 전체를 건너뛴다.
			reconcileSkipped = true
			log.Printf("issue ingest: reconcile 제거 생략 code=%s — 파싱 불가 파일 %d건", code, unparseable)
		} else {
			existing, _ := db.FindAllIssues(ctx, bson.M{"projectId": projectID})
			prefix := code + "-"
			for _, doc := range existing {
				k, _ := doc["key"].(string)
				// 이 과제 소속(key prefix=code)이면서 ingest 집합에 없으면 캐시에서 제거.
				// 단, 실패 파일의 키는 상태 불명이므로 보호.
				if k != "" && strings.HasPrefix(k, prefix) && !ingested[k] && !failedKeys[k] {
					if err := db.DeleteIssueByKey(ctx, k); err == nil {
						removed++
					}
				}
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ingested":         len(ingested),
		"removed":          removed,
		"reconcileSkipped": reconcileSkipped,
		"failed":           failed,
		"syncedAt":         time.Now(),
	})
}
