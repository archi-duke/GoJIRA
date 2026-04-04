package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"platform-api/db"
	"platform-api/github"
)

// === Users ===

func UserListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	users, err := db.FindAllUsers(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, users)
}

func UserGetHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	user, err := db.FindUserByID(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}
	respondJSON(w, http.StatusOK, user)
}

func UserCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.InsertUser(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Gitea에 사용자 자동 등록 (knoxId → username)
	if knoxId, _ := doc["knoxId"].(string); knoxId != "" {
		email, _ := doc["email"].(string)
		if email == "" {
			email = knoxId + "@gojira.local"
		}
		fullName, _ := doc["name"].(string)
		password := knoxId + "!2026"
		if err := github.CreateUser(knoxId, email, fullName, password); err != nil {
			log.Printf("Gitea 사용자 생성 실패 (%s): %v", knoxId, err)
		} else {
			log.Printf("Gitea 사용자 생성 완료: %s", knoxId)
			if err := github.AddUserToOrgTeam(knoxId); err != nil {
				log.Printf("Gitea 조직 멤버 추가 실패 (%s): %v", knoxId, err)
			}
		}
	}

	respondJSON(w, http.StatusOK, doc)
}

func UserUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var updates bson.M
	if err := decodeJSON(r, &updates); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.UpdateUser(ctx, id, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "user updated")
}

func UserDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.SoftDeleteUser(ctx, id, "system"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "user deactivated")
}

func UserAuthorityHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	auth, err := db.CalcUserAuthority(ctx, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, auth)
}

// 현재 로그인 사용자 정보 조회 (knoxId 기반)
func UserMeHandler(w http.ResponseWriter, r *http.Request) {
	knoxID := r.URL.Query().Get("knoxId")
	if knoxID == "" {
		respondError(w, http.StatusBadRequest, "knoxId is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	user, err := db.FindUserByKnoxID(ctx, knoxID)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}
	// 권한도 함께 반환
	userID, _ := user["_id"].(string)
	auth, _ := db.CalcUserAuthority(ctx, userID)
	respondJSON(w, http.StatusOK, bson.M{"user": user, "authority": auth})
}

// === AuthGroups ===

func AuthGroupListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	groups, err := db.FindAllAuthGroups(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, groups)
}

func AuthGroupCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.InsertAuthGroup(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

func AuthGroupUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var updates bson.M
	if err := decodeJSON(r, &updates); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.UpdateAuthGroup(ctx, id, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "authGroup updated")
}

func AuthGroupDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.SoftDeleteAuthGroup(ctx, id, "system"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "authGroup deactivated")
}

// === OrgUnits ===

func OrgUnitListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	units, err := db.FindAllOrgUnits(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, units)
}

func OrgUnitCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.InsertOrgUnit(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

func OrgUnitUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var updates bson.M
	if err := decodeJSON(r, &updates); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.UpdateOrgUnit(ctx, id, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "orgUnit updated")
}

func OrgUnitDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.SoftDeleteOrgUnit(ctx, id, "system"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "orgUnit deactivated")
}

// === UserProjects ===

func UserProjectListHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	ups, err := db.FindUserProjectsByUser(ctx, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, ups)
}

func UserProjectCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	doc["joinedAt"] = time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.InsertUserProject(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

func UserProjectDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.SoftDeleteUserProject(ctx, id, "system"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "userProject deactivated")
}

// === UserConnections ===

func ConnectionListHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	conns, err := db.FindConnectionsByUser(ctx, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, conns)
}

func ConnectionCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	doc["isOnline"] = true
	doc["connectedAt"] = time.Now()
	doc["lastSeenAt"] = time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.InsertConnection(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

func ConnectionUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var updates bson.M
	if err := decodeJSON(r, &updates); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.UpdateConnection(ctx, id, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "connection updated")
}
