package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	authmw "platform-api/auth"
	"platform-api/db"
	"platform-api/github"
)

// reqApp — 앱 스코프 쿼리 파라미터 (D8). 미지정은 'platform' (하위호환).
func reqApp(r *http.Request) string {
	return db.NormalizeApp(r.URL.Query().Get("app"))
}

// stripSecrets — 응답으로 내보낼 user 문서에서 비밀 필드를 제거한다.
// giteaToken(사용자 Gitea PAT)은 신원 증명이므로 일반 user 응답에 실려선 안 된다.
// 런처 등 신뢰 컴포넌트는 전용 엔드포인트(/Account/me/gitCredentials)로만 받는다.
// 입력 문서를 변형하지 않도록 얕은 복사 후 삭제한다.
func stripSecrets(user bson.M) bson.M {
	if user == nil {
		return nil
	}
	out := make(bson.M, len(user))
	for k, v := range user {
		if k == "giteaToken" {
			continue
		}
		out[k] = v
	}
	return out
}

// stripSecretsAll — user 문서 슬라이스에 stripSecrets 를 적용.
func stripSecretsAll(users []bson.M) []bson.M {
	out := make([]bson.M, 0, len(users))
	for _, u := range users {
		out = append(out, stripSecrets(u))
	}
	return out
}

// === Users ===

func UserListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	users, err := db.FindAllUsers(ctx, reqApp(r))
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, stripSecretsAll(users))
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
	respondJSON(w, http.StatusOK, stripSecrets(user))
}

func UserCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 저장 전 중복 검증
	knoxId, _ := doc["knoxId"].(string)
	email, _ := doc["email"].(string)
	if knoxId != "" {
		if exists, _ := db.KnoxIdExists(ctx, knoxId); exists {
			respondError(w, http.StatusConflict, fmt.Sprintf("Knox ID '%s'이(가) 이미 등록되어 있습니다.", knoxId))
			return
		}
	}
	if email != "" {
		if exists, _ := db.EmailExists(ctx, email); exists {
			respondError(w, http.StatusConflict, fmt.Sprintf("이메일 '%s'이(가) 이미 등록되어 있습니다. 다른 이메일을 입력하세요.", email))
			return
		}
		if knoxId != "" && github.CheckEmailExists(email) {
			respondError(w, http.StatusConflict, fmt.Sprintf("이메일 '%s'이(가) Git 플랫폼에서 이미 사용 중입니다. 다른 이메일을 입력하세요.", email))
			return
		}
	}

	if err := db.InsertUser(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Gitea에 사용자 자동 등록 (knoxId → username)
	if knoxId != "" {
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
			// Gitea API 토큰 생성 → DB 저장
			if token, err := github.CreateUserToken(knoxId, password); err != nil {
				log.Printf("Gitea 토큰 생성 실패 (%s): %v", knoxId, err)
			} else {
				db.UpdateUserGiteaToken(ctx, knoxId, token)
				log.Printf("Gitea 토큰 생성 완료: %s", knoxId)
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

	// 이메일 변경 시 중복 검증
	if email, ok := updates["email"].(string); ok && email != "" {
		if exists, _ := db.EmailExistsExcept(ctx, email, id); exists {
			respondError(w, http.StatusConflict, fmt.Sprintf("이메일 '%s'이(가) 이미 등록되어 있습니다. 다른 이메일을 입력하세요.", email))
			return
		}
		if github.CheckEmailExists(email) {
			// 본인 Gitea 계정의 이메일이면 허용
			user, _ := db.FindUserByID(ctx, id)
			knoxId, _ := user["knoxId"].(string)
			if !github.IsOwnEmail(knoxId, email) {
				respondError(w, http.StatusConflict, fmt.Sprintf("이메일 '%s'이(가) Git 플랫폼에서 이미 사용 중입니다. 다른 이메일을 입력하세요.", email))
				return
			}
		}
	}

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
	auth, err := db.CalcUserAuthority(ctx, id, reqApp(r))
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, auth)
}

// 현재 로그인 사용자 정보 조회 — 신원은 auth 미들웨어가 검증한 context 값만 사용(사칭 차단).
func UserMeHandler(w http.ResponseWriter, r *http.Request) {
	knoxID := authmw.UserID(r.Context())
	if knoxID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
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
	var userID string
	switch id := user["_id"].(type) {
	case bson.ObjectID:
		userID = id.Hex()
	case string:
		userID = id
	default:
		userID = fmt.Sprintf("%v", id)
	}
	auth, _ := db.CalcUserAuthority(ctx, userID, reqApp(r))
	respondJSON(w, http.StatusOK, bson.M{"user": stripSecrets(user), "authority": auth})
}

// UserGitCredentialsHandler — GET /Account/me/gitCredentials
//
// 요청자 **본인** 의 Gitea 자격(username + PAT)만 반환한다. 경로에 사용자 파라미터가
// 없으므로 타인 자격을 조회할 방법이 없다. 신원은 auth 미들웨어가 검증한 것(SSO
// Bearer 또는 X-Service-Token act-as)만 사용한다.
//
// 용도: 런처가 clone/fetch/push URL 에 임베드할 자격을 런타임에 받아간다.
// 이전에는 launcher.json 의 git.user/git.token 에 사람이 PAT 를 손으로 적어두어
// 전원이 공용 관리자 계정(gojira-admin)으로 push 되는 문제가 있었다.
//
// PAT 는 사용자 등록 시 Gitea 계정과 함께 자동 발급되어 users.giteaToken 에 저장된다
// (UserCreateHandler). 아직 없는 사용자는 404 — 호출 측은 기존 설정으로 폴백한다.
func UserGitCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	knoxID := authmw.UserID(r.Context())
	if knoxID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	token := db.GetUserGiteaToken(ctx, knoxID)
	if token == "" {
		respondError(w, http.StatusNotFound, fmt.Sprintf("'%s' 의 Gitea 토큰이 없습니다", knoxID))
		return
	}
	// 캐시 금지 — 자격증명이 중간 프록시/브라우저에 남지 않도록.
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, http.StatusOK, bson.M{"login": knoxID, "token": token})
}

// === AuthGroups ===

func AuthGroupListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	groups, err := db.FindAllAuthGroups(ctx, reqApp(r))
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
	// 앱 스코프 (D8) — body 의 app 우선, 없으면 쿼리 파라미터/기본값
	if _, ok := doc["app"].(string); !ok {
		doc["app"] = reqApp(r)
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
