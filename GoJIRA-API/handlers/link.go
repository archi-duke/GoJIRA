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
func saveLinkYamlFor(ctx context.Context, key, user, msg string) error {
	existing, err := db.FindIssueByKey(ctx, key)
	if err != nil || existing == nil {
		return fmt.Errorf("find %s: %w", key, err)
	}
	subkey, _ := existing["subkey"].(string)
	if subkey == "" {
		return nil
	}
	code := keyToProjectCode(key)
	if code == "" {
		return fmt.Errorf("cannot derive project code from %s", key)
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
		return fmt.Errorf("yaml marshal %s: %w", key, err)
	}
	if err := clients.SaveFile(code, issueYamlPath(subkey), string(yamlBytes), msg, user); err != nil {
		return fmt.Errorf("save yaml %s: %w", key, err)
	}

	merged["_dirty"] = false
	merged["_syncedAt"] = time.Now()
	if err := db.UpsertIssue(ctx, key, merged); err != nil {
		log.Printf("cache upsert warn key=%s: %v", key, err)
	}
	return nil
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
	if err := db.AddIssueLink(ctx, req.OutwardKey, outwardLink); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	msg := fmt.Sprintf("[link] create %s → %s", req.InwardKey, req.OutwardKey)
	if err := saveLinkYamlFor(ctx, req.InwardKey, user, msg); err != nil {
		log.Printf("link yaml save warn (inward %s): %v", req.InwardKey, err)
	}
	if err := saveLinkYamlFor(ctx, req.OutwardKey, user, msg); err != nil {
		log.Printf("link yaml save warn (outward %s): %v", req.OutwardKey, err)
	}

	db.SyncIssueToJira(req.InwardKey)
	respondJSON(w, http.StatusOK, "link created")
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
	for _, k := range keys {
		if err := saveLinkYamlFor(ctx, k, user, msg); err != nil {
			log.Printf("link yaml save warn (%s): %v", k, err)
		}
		db.SyncIssueToJira(k)
	}
	respondJSON(w, http.StatusOK, "link deleted")
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
	for _, k := range keys {
		if err := saveLinkYamlFor(ctx, k, user, msg); err != nil {
			log.Printf("link yaml save warn (%s): %v", k, err)
		}
		db.SyncIssueToJira(k)
	}
	respondJSON(w, http.StatusOK, "link deleted")
}
