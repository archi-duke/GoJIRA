package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"gojira-api/auth"
	"gojira-api/config"
	"gojira-api/db"
	"gojira-api/handlers"
)

const trashRetentionDays = 3

// purgeOldTrashWorker — 1시간마다 issues_trash 의 _trashedAt 이 retention 초과한 항목 영구 삭제.
func purgeOldTrashWorker() {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cutoff := time.Now().Add(-time.Duration(trashRetentionDays) * 24 * time.Hour)
		if n, err := db.PurgeOldTrash(ctx, cutoff); err != nil {
			log.Printf("trash purge error: %v", err)
		} else if n > 0 {
			log.Printf("trash purge: %d 건 영구 삭제 (retention=%dd)", n, trashRetentionDays)
		}
		cancel()
		time.Sleep(1 * time.Hour)
	}
}

// issueSubSegments — /Issue 바로 다음에 오는 하위 세그먼트(동적 파라미터 없이 고정 위치).
// Project 류는 다음 세그먼트가 {jiraCode} 실값이라 정규화 대상에서 제외한다 — 잘못 건드리면
// 프로젝트 코드 자체가 손상된다.
var issueSubSegments = map[string]string{
	"ingest":    "ingest",
	"label":     "label",
	"link":      "link",
	"comment":   "comment",
	"fetch":     "fetch",
	"field":     "field",
	"relations": "relations",
}

func caseInsensitiveRouting(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		canonical := map[string]string{
			"check":   "Check",
			"issue":   "Issue",
			"project": "Project",
			"jira":    "Jira",
			"page":    "Page",
			"agent":   "Agent",
		}
		if len(parts) >= 4 {
			if mapped, ok := canonical[strings.ToLower(parts[3])]; ok {
				parts[3] = mapped
			}
			if parts[3] == "Issue" && len(parts) >= 5 {
				if mapped, ok := issueSubSegments[strings.ToLower(parts[4])]; ok {
					parts[4] = mapped
				}
			}
			r.URL.Path = strings.Join(parts, "/")
		}
		next.ServeHTTP(w, r)
	})
}

// notFoundHint — 에이전트(특히 URL 을 기억/추측해 curl 을 직접 만드는 모델)가 잘못된 경로를
// 호출했을 때 맨 "404 page not found" 대신 올바른 사용법을 안내한다. 이슈 조회는 애초에
// 이 API 를 직접 두드리는 대신 issue-query.sh(로컬 clone 읽기)를 써야 한다 — Mongo 는
// PR 머지 시점에만 갱신되므로 세션 중 조회 결과가 비어있을 수 있다.
func notFoundHint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, `{"error":"unknown route","path":%q,"hint":"issue 조회는 GoJIRA-API 를 직접 두드리지 말고 issue-query.sh(로컬 clone 읽기)를 사용하세요. API 직접 호출이 꼭 필요하면 경로 대소문자를 확인: GET/POST/PUT/DELETE /api/v2/Issue, /api/v2/Issue/{ingest|label|link|comment|fetch|field|relations}, /api/v2/Project"}`, r.URL.Path)
}

func main() {
	db.Init()

	// Trash sweeper — 휴지통 (issues_trash) 의 retention 초과 항목 자동 영구 삭제
	go purgeOldTrashWorker()

	r := chi.NewRouter()
	r.NotFound(notFoundHint)
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
	// SSO 인증 — 검증된 신원을 context 주입(핸들러는 auth.UserID). 비인증 경로·OPTIONS 통과.
	// USE_SSO!=true(dev)면 X-User-Id/DEV_USER 폴백이라 동작 무변.
	r.Use(auth.Middleware)

	r.Route("/api/v{version}", func(r chi.Router) {
		// Check
		r.Get("/Check", handlers.CheckHandler)

		// Issue CRUD
		r.Post("/Issue", handlers.IssueCreateHandler)
		r.Get("/Issue", handlers.IssueGetHandler)
		r.Put("/Issue", handlers.IssueUpdateHandler)
		r.Delete("/Issue", handlers.IssueDeleteHandler)

		// Issue ingest (트랙 E — 로컬 YAML → Mongo 캐시)
		r.Post("/Issue/ingest", handlers.IssueIngestHandler)
		r.Delete("/Issue/ingest", handlers.IssueIngestDeleteHandler)
		r.Post("/Project/{jiraCode}/issues/ingest", handlers.ProjectIssuesIngestHandler)

		// Label
		r.Post("/Issue/label", handlers.LabelCreateHandler)
		r.Delete("/Issue/label", handlers.LabelDeleteHandler)

		// Link
		r.Post("/Issue/link", handlers.LinkCreateHandler)
		r.Delete("/Issue/link", handlers.LinkDeleteHandler)
		r.Delete("/Issue/link/{linkId}/of/{key}", handlers.LinkDeleteByKeyHandler)

		// Comment
		r.Post("/Issue/comment", handlers.CommentCreateHandler)
		r.Put("/Issue/comment", handlers.CommentUpdateHandler)
		r.Delete("/Issue/comment", handlers.CommentDeleteHandler)

		// Fetch (pull from JIRA)
		r.Get("/Issue/fetch", handlers.IssueFetchHandler)

		// Field
		r.Put("/Issue/field", handlers.FieldUpdateHandler)

		// Relations
		r.Put("/Issue/relations", handlers.RelationsHandler)

		// Jira Account
		r.Post("/Jira/Account", handlers.AccountCreateHandler)

		// Page
		r.Post("/Page/images", handlers.PageImagesPostHandler)
		r.Put("/Page/images", handlers.PageImagesPutHandler)

		// Project
		r.Post("/Project", handlers.ProjectCreateHandler)
		r.Get("/Project", handlers.ProjectGetHandler)
		r.Put("/Project", handlers.ProjectUpdateHandler)
		r.Patch("/Project/{jiraCode}/archSettings", handlers.ProjectArchSettingsPatchHandler)
		r.Post("/Project/{jiraCode}/reset", handlers.ProjectResetHandler)
		r.Get("/Project/{jiraCode}/dump", handlers.ProjectDumpHandler)
		r.Get("/Project/{jiraCode}/usecase-artifacts", handlers.UsecaseArtifactGetHandler)
		r.Put("/Project/{jiraCode}/usecase-artifacts", handlers.UsecaseArtifactPutHandler)
		r.Get("/Project/alerts", handlers.ProjectAlertsHandler)
	})

	fmt.Printf("─── GoJIRA-API ─────────────────────────\n")
	fmt.Printf("LISTEN            : %s\n", config.ServerPort)
	fmt.Printf("MongoDB           : %s/%s\n", config.MongoURI, config.MongoDB)
	fmt.Printf("JIRA Mock         : %s\n", config.JiraMockURL)
	fmt.Printf("Platform-API peer : %s\n", config.PlatformAPI)
	fmt.Printf("Ports overview    : docs/ports.md\n")
	fmt.Printf("─────────────────────────────────────────\n")
	log.Fatal(http.ListenAndServe(config.ServerPort, r))
}
