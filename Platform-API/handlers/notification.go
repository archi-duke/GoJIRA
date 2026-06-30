package handlers

import (
	"context"
	"net/http"
	"time"

	authmw "platform-api/auth"
	"platform-api/db"
)

// GET /Notification/unread?user={userId}
func NotificationUnreadHandler(w http.ResponseWriter, r *http.Request) {
	userId := authmw.UserID(r.Context())
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
	userId := authmw.UserID(r.Context())
	if userId == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	db.MarkNotificationsRead(ctx, userId)
	respondJSON(w, http.StatusOK, "ok")
}
