package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keys, err := db.RemoveIssueLink(ctx, req.LinkID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, k := range keys {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keys, err := db.RemoveIssueLink(ctx, linkID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, k := range keys {
		db.SyncIssueToJira(k)
	}
	respondJSON(w, http.StatusOK, "link deleted")
}
