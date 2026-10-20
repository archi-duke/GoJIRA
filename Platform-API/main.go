package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"platform-api/auth"
	"platform-api/config"
	"platform-api/db"
	"platform-api/handlers"
	"platform-api/notify"
)

func caseInsensitiveRouting(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(r.URL.Path, "/", 5)
		canonical := map[string]string{
			"account":  "Account",
			"notice":   "Notice",
			"docrepo":  "DocRepo",
			"doclink":  "DocLink",
			"git":      "Git",
		}
		if len(parts) >= 4 {
			if mapped, ok := canonical[strings.ToLower(parts[3])]; ok {
				parts[3] = mapped
			}
			r.URL.Path = strings.Join(parts, "/")
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	db.Init()
	db.InitAccountIndexes()
	db.InitNoticeIndexes()
	db.InitDocRepoIndexes()
	db.InitNotificationIndexes()
	db.SeedAccountData()

	// 알림 provider 연결 — NOTIFY_PROVIDER 설정에 따라 smtp/internal/none 선택
	notify.Init()

	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(caseInsensitiveRouting)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// SSO 인증 — 검증된 신원을 context 주입(핸들러는 auth.UserID). 비인증 경로(/launcher,
	// /webhook)·OPTIONS 통과. USE_SSO!=true(dev)면 X-User-Id/DEV_USER 폴백이라 동작 무변.
	r.Use(auth.Middleware)

	// Webhook (버전 없이)
	r.Post("/webhook/github", handlers.GitHubWebhookHandler)

	r.Route("/api/v{version}", func(r chi.Router) {
		// Account 관리
		r.Route("/Account", func(r chi.Router) {
			r.Get("/me", handlers.UserMeHandler)
			// 본인 Gitea 자격(username+PAT) — 런처가 clone/push URL 용으로 조회.
			r.Get("/me/gitCredentials", handlers.UserGitCredentialsHandler)

			r.Get("/users", handlers.UserListHandler)
			r.Get("/users/{id}", handlers.UserGetHandler)
			r.Post("/users", handlers.UserCreateHandler)
			r.Put("/users/{id}", handlers.UserUpdateHandler)
			r.Delete("/users/{id}", handlers.UserDeleteHandler)
			r.Get("/users/{id}/authority", handlers.UserAuthorityHandler)

			r.Get("/authGroups", handlers.AuthGroupListHandler)
			r.Post("/authGroups", handlers.AuthGroupCreateHandler)
			r.Put("/authGroups/{id}", handlers.AuthGroupUpdateHandler)
			r.Delete("/authGroups/{id}", handlers.AuthGroupDeleteHandler)

			r.Get("/orgUnits", handlers.OrgUnitListHandler)
			r.Post("/orgUnits", handlers.OrgUnitCreateHandler)
			r.Put("/orgUnits/{id}", handlers.OrgUnitUpdateHandler)
			r.Delete("/orgUnits/{id}", handlers.OrgUnitDeleteHandler)

			r.Get("/userProjects", handlers.UserProjectListHandler)
			r.Post("/userProjects", handlers.UserProjectCreateHandler)
			r.Delete("/userProjects/{id}", handlers.UserProjectDeleteHandler)

			r.Get("/connections", handlers.ConnectionListHandler)
			r.Post("/connections", handlers.ConnectionCreateHandler)
			r.Put("/connections/{id}", handlers.ConnectionUpdateHandler)
		})

		// Notifications (사용자별 알림)
		r.Get("/Notification/unread", handlers.NotificationUnreadHandler)
		r.Post("/Notification/read", handlers.NotificationReadHandler)

		// Notice
		r.Get("/Notice/stream", handlers.NoticeStreamHandler)
		r.Get("/Notice/ws", handlers.NoticeWSHandler)
		r.Get("/Notice/active", handlers.NoticeActiveHandler)
		r.Get("/Notice", handlers.NoticeListHandler)
		r.Get("/Notice/{id}", handlers.NoticeGetHandler)
		r.Post("/Notice", handlers.NoticeCreateHandler)
		r.Put("/Notice/{id}", handlers.NoticeUpdateHandler)
		r.Delete("/Notice/{id}", handlers.NoticeDeleteHandler)
		r.Post("/Notice/{id}/view", handlers.NoticeViewHandler)

		// Git 플랫폼 사용자 목록
		r.Get("/Git/users", handlers.GitUsersHandler)

		// Project archive (zip 백업 + 모든 자원 정리)
		r.Post("/Project/{code}/archive", handlers.ProjectArchiveHandler)

		// LLM gateway — Platform-API 가 키/모델/엔드포인트를 단독 관리, 다른 서비스는 HTTP 호출
		r.Post("/LLM/call", handlers.LLMCallHandler)
		// LLM 연결 검증용 단순 질의응답 (curl/브라우저 확인용)
		r.Get("/LLM/chat", handlers.LLMChatHandler)
		r.Post("/LLM/chat", handlers.LLMChatHandler)

		// Launcher 바이너리 호스팅 — 웹이 런처 미감지 상태에서 다운로드.
		r.Get("/launcher/manifest", handlers.LauncherManifestHandler)
		r.Get("/launcher/version", handlers.LauncherVersionHandler)
		r.Get("/launcher/download/{os}", handlers.LauncherDownloadHandler)

		// DocRepo — 문서 이력관리 (projectCode 기반)
		r.Route("/DocRepo", func(r chi.Router) {
			r.Get("/", handlers.DocRepoListHandler)
			r.Get("/check", handlers.DocRepoCheckHandler)
			r.Post("/sync", handlers.DocRepoSyncHandler)
			r.Post("/{code}", handlers.DocRepoCreateHandler)
			r.Delete("/{code}/purge", handlers.DocRepoPurgeHandler)
			r.Delete("/{code}/github", handlers.DocRepoGHDeleteHandler)
			r.Get("/{code}/tree", handlers.DocRepoTreeHandler)
			r.Get("/{code}/file", handlers.DocRepoFileHandler)
			r.Get("/{code}/commits", handlers.DocRepoCommitsHandler)
			r.Get("/{code}/branches", handlers.DocRepoBranchesHandler)
			r.Post("/{code}/alerts/recompute", handlers.DocRepoAlertsRecomputeHandler)
			r.Post("/{code}/issues/resync", handlers.DocRepoIssuesResyncHandler)
			r.Get("/{code}/graph", handlers.DocRepoGraphHandler)
			r.Post("/{code}/pr", handlers.DocRepoPRCreateHandler)
			r.Get("/{code}/prs", handlers.DocRepoPRListHandler)
			r.Post("/{code}/pr/{num}/review", handlers.DocRepoPRReviewHandler)
			r.Post("/{code}/pr/{num}/merge", handlers.DocRepoPRMergeHandler)
			r.Get("/{code}/pr/{num}/reviews", handlers.DocRepoPRReviewsHandler)
		})

		// DocLink — 향후 구현
	})

	fmt.Printf("─── Platform-API ───────────────────────\n")
	fmt.Printf("LISTEN          : %s\n", config.ServerPort)
	fmt.Printf("MongoDB         : %s/%s\n", config.MongoURI, config.MongoDB)
	fmt.Printf("Gitea (GitHub)  : %s\n", config.GitHubURL)
	fmt.Printf("GoJIRA-API peer : %s\n", config.GoJiraAPI)
	fmt.Printf("Notify provider : %s\n", notify.Provider())
	fmt.Printf("Ports overview  : docs/ports.md\n")
	fmt.Printf("─────────────────────────────────────────\n")
	log.Fatal(http.ListenAndServe(config.ServerPort, r))
}
