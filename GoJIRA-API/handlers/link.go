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

	"gojira-api/clients"
	"gojira-api/db"
	"gojira-api/models"
)

// saveLinkYamlFor reads the full issue from Mongo and writes its YAML file,
// preserving issueLinks. Returns nil if the issue has no subkey (YAML path
// cannot be derived) — Mongo-only state is tolerated.
func saveLinkYamlFor(ctx context.Context, key, user, msg string, local bool) (*localWrite, error) {
	existing, err := db.FindIssueByKey(ctx, key)
	if err != nil || existing == nil {
		return nil, fmt.Errorf("find %s: %w", key, err)
	}
	subkey, _ := existing["subkey"].(string)
	if subkey == "" {
		return nil, nil
	}
	code := keyToProjectCode(key)
	if code == "" {
		return nil, fmt.Errorf("cannot derive project code from %s", key)
	}

	merged := bson.M{}
	for k, v := range existing {
		if k == "_id" || k == "_dirty" || k == "_syncedAt" {
			continue
		}
		merged[k] = v
	}
	merged["updated"] = time.Now().Format("2006-01-02T15:04:05.000+0900")

	yamlBytes, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("yaml marshal %s: %w", key, err)
	}

	var lw *localWrite
	if local {
		lw = &localWrite{Path: issueYamlPath(subkey), Content: string(yamlBytes)}
	} else if err := clients.SaveFile(code, issueYamlPath(subkey), string(yamlBytes), msg, user); err != nil {
		return nil, fmt.Errorf("save yaml %s: %w", key, err)
	}

	merged["_dirty"] = false
	merged["_syncedAt"] = time.Now()
	if err := db.UpsertIssue(ctx, key, merged); err != nil {
		log.Printf("cache upsert warn key=%s: %v", key, err)
	}
	return lw, nil
}

// collectLinkWrites — 여러 key 의 link YAML 을 저장/수집한다.
// local=true 면 localWrite 목록을 모아 반환(서버 fs 미사용), false 면 즉시 worktree 커밋.
func collectLinkWrites(ctx context.Context, keys []string, user, msg string, local bool) []localWrite {
	var writes []localWrite
	for _, k := range keys {
		lw, err := saveLinkYamlFor(ctx, k, user, msg, local)
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
	user, ok := resolveUser(r, "")
	if !ok {
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

	msg := fmt.Sprintf("[link] create %s → %s", req.InwardKey, req.OutwardKey)
	writes := collectLinkWrites(ctx, []string{req.InwardKey, req.OutwardKey}, user, msg, isLocalWrite(r))

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
	user, ok := resolveUser(r, "")
	if !ok {
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
	msg := fmt.Sprintf("[link] delete %s", req.LinkID)
	writes := collectLinkWrites(ctx, keys, user, msg, isLocalWrite(r))
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
	user, ok := resolveUser(r, "")
	if !ok {
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
	msg := fmt.Sprintf("[link] delete %s", linkID)
	writes := collectLinkWrites(ctx, keys, user, msg, isLocalWrite(r))
	for _, k := range keys {
		db.SyncIssueToJira(k)
	}
	resp := map[string]interface{}{"status": "link deleted"}
	if writes != nil {
		resp["localWrites"] = writes
	}
	respondJSON(w, http.StatusOK, resp)
}
