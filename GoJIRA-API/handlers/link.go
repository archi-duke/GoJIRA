package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"

	"gojira-api/db"
	"gojira-api/models"
)

// saveLinkYamlFor reads the full issue from Mongo and writes its YAML file,
// preserving issueLinks. Returns nil if the issue has no subkey (YAML path
// cannot be derived) — Mongo-only state is tolerated.
func saveLinkYamlFor(ctx context.Context, key string) (*localWrite, error) {
	existing, err := db.FindIssueByKey(ctx, key)
	if err != nil || existing == nil {
		return nil, fmt.Errorf("find %s: %w", key, err)
	}
	subkey, _ := existing["subkey"].(string)
	if subkey == "" {
		return nil, nil
	}

	merged := bson.M{}
	for k, v := range existing {
		if k == "_id" || k == "_dirty" || k == "_syncedAt" {
			continue
		}
		merged[k] = v
	}
	merged["updated"] = time.Now().Format("2006-01-02T15:04:05.000+0900")

	// NBBALL 사고 재발 방지: Mongo 조회 결과의 중첩 bson.D 를 순수 맵/배열로
	// 정규화한 뒤 직렬화한다 (bson_normalize.go 주석 참고).
	yamlBytes, err := yaml.Marshal(normalizeBsonDoc(merged))
	if err != nil {
		return nil, fmt.Errorf("yaml marshal %s: %w", key, err)
	}

	// 갱신된 이슈 YAML 을 로컬 clone 에 기록 지시 — 호출자가 적용. 서버는 fs 미접촉.
	lw := &localWrite{Path: issueYamlPath(subkey), Content: string(yamlBytes)}

	merged["_dirty"] = false
	merged["_syncedAt"] = time.Now()
	if err := db.UpsertIssue(ctx, key, merged); err != nil {
		log.Printf("cache upsert warn key=%s: %v", key, err)
	}
	return lw, nil
}

// collectLinkWrites — 여러 key 의 link YAML 갱신을 localWrite 목록으로 모은다(서버 fs 미사용).
func collectLinkWrites(ctx context.Context, keys []string) []localWrite {
	var writes []localWrite
	for _, k := range keys {
		lw, err := saveLinkYamlFor(ctx, k)
		if err != nil {
			log.Printf("link yaml save warn (%s): %v", k, err)
			continue
		}
		if lw != nil {
			writes = append(writes, *lw)
		}
	}
	return writes
}

// POST /api/v{version}/Issue/link
func LinkCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.LinkCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.InwardKey == "" || req.OutwardKey == "" {
		respondError(w, http.StatusBadRequest, "inwardKey and outwardKey are required")
		return
	}
	if _, ok := resolveUser(r, ""); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Generate a temporary link ID
	linkID := fmt.Sprintf("tmp_%d", time.Now().UnixMilli())

	// Add outwardIssue link to inward issue's issueLinks
	inwardLink := bson.M{
		"id":           linkID,
		"type":         bson.M{"name": "Contains(WBSGantt)"},
		"outwardIssue": bson.M{"key": req.OutwardKey},
	}
	if req.Source != "" {
		inwardLink["_addedBy"] = req.Source
	}
	if err := db.AddIssueLink(ctx, req.InwardKey, inwardLink); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Add inwardIssue link to outward issue's issueLinks
	outwardLink := bson.M{
		"id":          linkID,
		"type":        bson.M{"name": "Contains(WBSGantt)"},
		"inwardIssue": bson.M{"key": req.InwardKey},
	}
	if req.Source != "" {
		outwardLink["_addedBy"] = req.Source
	}
	if err := db.AddIssueLink(ctx, req.OutwardKey, outwardLink); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writes := collectLinkWrites(ctx, []string{req.InwardKey, req.OutwardKey})

	db.SyncIssueToJira(req.InwardKey)
	resp := map[string]interface{}{"status": "link created"}
	if writes != nil {
		resp["localWrites"] = writes
	}
	respondJSON(w, http.StatusOK, resp)
}

// DELETE /api/v{version}/Issue/link
func LinkDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req models.LinkDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.LinkID == "" {
		respondError(w, http.StatusBadRequest, "linkId is required")
		return
	}
	if _, ok := resolveUser(r, ""); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keys, err := db.RemoveIssueLink(ctx, req.LinkID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writes := collectLinkWrites(ctx, keys)
	for _, k := range keys {
		db.SyncIssueToJira(k)
	}
	resp := map[string]interface{}{"status": "link deleted"}
	if writes != nil {
		resp["localWrites"] = writes
	}
	respondJSON(w, http.StatusOK, resp)
}

// DELETE /api/v{version}/Issue/link/{linkId}/of/{key}
func LinkDeleteByKeyHandler(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkId")
	if linkID == "" {
		respondError(w, http.StatusBadRequest, "linkId is required")
		return
	}
	if _, ok := resolveUser(r, ""); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keys, err := db.RemoveIssueLink(ctx, linkID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writes := collectLinkWrites(ctx, keys)
	for _, k := range keys {
		db.SyncIssueToJira(k)
	}
	resp := map[string]interface{}{"status": "link deleted"}
	if writes != nil {
		resp["localWrites"] = writes
	}
	respondJSON(w, http.StatusOK, resp)
}
