package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"context"
	"time"

	"platform-api/config"
	"platform-api/db"
	gitpkg "platform-api/github"
	"platform-api/handlers"
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
	db.InitEditSessionIndexes()
	db.InitOpencodeInstanceIndexes()
	db.InitNotificationIndexes()
	db.SeedAccountData()

	// OpenCode 인스턴스 헬스체크 — 5분 주기, status=active/down 갱신
	go handlers.StartOpencodeHealthcheck(context.Background())

	// Workspace TTL worker 시작
	gitpkg.StartTTLWorker(func(owner, repo, userID string) bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// projectCode는 repo name과 동일하다고 가정 (docrepo 매핑)
		session, err := db.FindActiveSessionByUser(ctx, repo, userID)
		return err == nil && session != nil
	})

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

	// Webhook (버전 없이)
	r.Post("/webhook/github", handlers.GitHubWebhookHandler)

	// 관리자 트리거 — 헬스체크 즉시 실행
	r.Post("/admin/opencode-healthcheck", handlers.RunOpencodeHealthcheckHandler)

	r.Route("/api/v{version}", func(r chi.Router) {
		// Account 관리
		r.Route("/Account", func(r chi.Router) {
			r.Get("/me", handlers.UserMeHandler)

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

		// DocRepo — 문서 이력관리 (projectCode 기반)
		r.Route("/DocRepo", func(r chi.Router) {
			r.Get("/", handlers.DocRepoListHandler)
			r.Post("/sync", handlers.DocRepoSyncHandler)
			r.Post("/{code}", handlers.DocRepoCreateHandler)
			r.Delete("/{code}/purge", handlers.DocRepoPurgeHandler)
			r.Delete("/{code}/github", handlers.DocRepoGHDeleteHandler)
			r.Post("/{code}/init-files", handlers.DocRepoInitFilesHandler)
			r.Post("/{code}/ensure-branch", handlers.DocRepoEnsureBranchHandler)
			r.Get("/{code}/worktree-info", handlers.DocRepoWorktreeInfoHandler)
			r.Post("/{code}/notify-change", handlers.DocRepoNotifyChangeHandler)
			r.Post("/{code}/sync-branch", handlers.DocRepoSyncBranchHandler)
			r.Get("/{code}/tree", handlers.DocRepoTreeHandler)
			r.Get("/{code}/file", handlers.DocRepoFileHandler)
			r.Put("/{code}/file", handlers.DocRepoFileUpdateHandler)
			r.Delete("/{code}/file", handlers.DocRepoFileDeleteHandler)
			r.Get("/{code}/commits", handlers.DocRepoCommitsHandler)
			r.Get("/{code}/branches", handlers.DocRepoBranchesHandler)
			r.Get("/{code}/graph", handlers.DocRepoGraphHandler)
			r.Post("/{code}/pr", handlers.DocRepoPRCreateHandler)
			r.Get("/{code}/prs", handlers.DocRepoPRListHandler)
			r.Post("/{code}/pr/{num}/review", handlers.DocRepoPRReviewHandler)
			r.Post("/{code}/pr/{num}/merge", handlers.DocRepoPRMergeHandler)
			r.Get("/{code}/pr/{num}/reviews", handlers.DocRepoPRReviewsHandler)
			r.Post("/{code}/git/publish", handlers.DocRepoGitPublishHandler)
			// 문서 편집 API
			r.Post("/{code}/edit/start", handlers.DocRepoEditStartHandler)
			r.Get("/{code}/edit/file", handlers.DocRepoEditFileHandler)
			r.Put("/{code}/edit/file", handlers.DocRepoEditSaveHandler)
			r.Post("/{code}/edit/commit", handlers.DocRepoEditCommitHandler)
			r.Post("/{code}/edit/pr", handlers.DocRepoEditPRHandler)
			r.Get("/{code}/edit/status", handlers.DocRepoEditStatusHandler)
			r.Delete("/{code}/edit/discard", handlers.DocRepoEditDiscardHandler)
			r.Get("/{code}/edit/pending", handlers.DocRepoEditPendingHandler)
			r.Post("/{code}/edit/heartbeat", handlers.DocRepoEditHeartbeatHandler)
			r.Post("/{code}/edit/release", handlers.DocRepoEditReleaseHandler)
			r.Post("/{code}/edit/rebase", handlers.DocRepoEditRebaseHandler)
			r.Post("/{code}/conflict-resolve", handlers.DocRepoConflictResolveHandler)
			r.Post("/{code}/open-folder", handlers.DocRepoOpenFolderHandler)
		})

		// DocLink — 향후 구현
	})

	fmt.Printf("─── Platform-API ───────────────────────\n")
	fmt.Printf("LISTEN          : %s\n", config.ServerPort)
	fmt.Printf("MongoDB         : %s/%s\n", config.MongoURI, config.MongoDB)
	fmt.Printf("Gitea (GitHub)  : %s\n", config.GitHubURL)
	fmt.Printf("GoJIRA-API peer : %s\n", config.GoJiraAPI)
	fmt.Printf("Ports overview  : docs/ports.md\n")
	fmt.Printf("─────────────────────────────────────────\n")
	log.Fatal(http.ListenAndServe(config.ServerPort, r))
}
