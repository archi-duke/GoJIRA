package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

var commentIDSeq int64 = 1000

// POST /api/v{version}/Issue/comment
func CommentCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.CommentRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" || req.Body == "" {
		respondError(w, http.StatusBadRequest, "key and body are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	id := fmt.Sprintf("%d", atomic.AddInt64(&commentIDSeq, 1))
	doc := bson.M{
		"commentId": id,
		"issueKey":  req.Key,
		"body":      req.Body,
		"created":   time.Now(),
	}

	if err := db.InsertComment(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

// PUT /api/v{version}/Issue/comment
func CommentUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.CommentRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" || req.CommentID == "" || req.Body == "" {
		respondError(w, http.StatusBadRequest, "key, commentId, and body are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := db.UpdateComment(ctx, req.CommentID, req.Body); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "comment updated")
}

// DELETE /api/v{version}/Issue/comment
func CommentDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req models.CommentDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" || req.CommentID == "" {
		respondError(w, http.StatusBadRequest, "key and commentId are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := db.DeleteComment(ctx, req.CommentID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "comment deleted")
}
