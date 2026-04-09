package handlers

import (
	"context"
	"net/http"
	"time"

	"platform-api/db"
)

// GET /Notification/unread?user={userId}
func NotificationUnreadHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.URL.Query().Get("user")
	if userId == "" {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	notifs, err := db.FindUnreadNotifications(ctx, userId)
	if err != nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	respondJSON(w, http.StatusOK, notifs)
}

// POST /Notification/read?user={userId}
func NotificationReadHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.URL.Query().Get("user")
	if userId == "" {
		respondError(w, http.StatusBadRequest, "user 필요")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	db.MarkNotificationsRead(ctx, userId)
	respondJSON(w, http.StatusOK, "ok")
}
