package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
)

// GET /Notice — 전체 공지 목록 (관리용)
func NoticeListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 상태 자동 갱신
	db.UpdateNoticeStatuses(ctx)

	notices, err := db.FindAllNotices(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, notices)
}

// GET /Notice/{id} — 단건 조회
func NoticeGetHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	notice, err := db.FindNoticeByID(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "notice not found")
		return
	}
	respondJSON(w, http.StatusOK, notice)
}

// GET /Notice/active — 현재 게시중인 공지 (사용자 팝업용)
func NoticeActiveHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	db.UpdateNoticeStatuses(ctx)

	notices, err := db.FindActiveNotices(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, notices)
}

// POST /Notice — 공지 생성
func NoticeCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// 날짜 문자열 → time.Time 변환
	if s, ok := doc["startDate"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			doc["startDate"] = t
		}
	}
	if s, ok := doc["endDate"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			doc["endDate"] = t
		}
	}
	if doc["status"] == nil {
		doc["status"] = "draft"
	}
	if doc["views"] == nil {
		doc["views"] = 0
	}
	if doc["bodyType"] == nil {
		doc["bodyType"] = "markdown"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.InsertNotice(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

// PUT /Notice/{id} — 공지 수정
func NoticeUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var updates bson.M
	if err := decodeJSON(r, &updates); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if s, ok := updates["startDate"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			updates["startDate"] = t
		}
	}
	if s, ok := updates["endDate"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			updates["endDate"] = t
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.UpdateNotice(ctx, id, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 비상 공지 활성화 시 SSE 브로드캐스트
	if isEmg, ok := updates["isEmergency"].(bool); ok && isEmg {
		notice, _ := db.FindNoticeByID(ctx, id)
		if notice != nil {
			BroadcastEmergencyNotice(notice)
		}
	}

	respondJSON(w, http.StatusOK, "notice updated")
}

// DELETE /Notice/{id} — 공지 삭제 (soft delete)
func NoticeDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.SoftDeleteNotice(ctx, id, "system"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "notice deleted")
}

// POST /Notice/{id}/view — 조회수 증가
func NoticeViewHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.IncrementNoticeViews(ctx, id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "view counted")
}
