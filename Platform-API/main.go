package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"platform-api/config"
	"platform-api/db"
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
	db.SeedAccountData()

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

		// Notice
		r.Get("/Notice/stream", handlers.NoticeStreamHandler)
		r.Get("/Notice/active", handlers.NoticeActiveHandler)
		r.Get("/Notice", handlers.NoticeListHandler)
		r.Get("/Notice/{id}", handlers.NoticeGetHandler)
		r.Post("/Notice", handlers.NoticeCreateHandler)
		r.Put("/Notice/{id}", handlers.NoticeUpdateHandler)
		r.Delete("/Notice/{id}", handlers.NoticeDeleteHandler)
		r.Post("/Notice/{id}/view", handlers.NoticeViewHandler)

		// DocRepo — 문서 이력관리
		r.Post("/DocRepo", handlers.DocRepoCreateHandler)
		r.Get("/DocRepo", handlers.DocRepoListHandler)
		r.Post("/DocRepo/sync", handlers.DocRepoSyncHandler)
		r.Put("/DocRepo/{id}", handlers.DocRepoUpdateHandler)
		r.Delete("/DocRepo/{id}", handlers.DocRepoDeleteHandler)
		r.Delete("/DocRepo/{id}/purge", handlers.DocRepoPurgeHandler)
		r.Get("/DocRepo/{id}/tree", handlers.DocRepoTreeHandler)
		r.Get("/DocRepo/{id}/file", handlers.DocRepoFileHandler)
		r.Get("/DocRepo/{id}/commits", handlers.DocRepoCommitsHandler)
		r.Get("/DocRepo/{id}/branches", handlers.DocRepoBranchesHandler)
		r.Post("/DocRepo/{id}/pr", handlers.DocRepoPRCreateHandler)
		r.Get("/DocRepo/{id}/prs", handlers.DocRepoPRListHandler)
		r.Post("/DocRepo/{id}/pr/{num}/review", handlers.DocRepoPRReviewHandler)
		r.Post("/DocRepo/{id}/pr/{num}/merge", handlers.DocRepoPRMergeHandler)
		r.Get("/DocRepo/{id}/pr/{num}/reviews", handlers.DocRepoPRReviewsHandler)
		r.Put("/DocRepo/{id}/file", handlers.DocRepoFilePutHandler)
		r.Post("/DocRepo/{id}/branch", handlers.DocRepoBranchCreateHandler)

		// Git 로컬 작업
		r.Post("/DocRepo/{id}/clone", handlers.DocRepoCloneHandler)
		r.Post("/DocRepo/{id}/git/write", handlers.DocRepoGitWriteHandler)
		r.Post("/DocRepo/{id}/git/publish", handlers.DocRepoGitPublishHandler)

		// DocLink — 문서-코드 연결
		r.Post("/DocLink", handlers.DocLinkCreateHandler)
		r.Get("/DocLink", handlers.DocLinkListHandler)
		r.Delete("/DocLink/{id}", handlers.DocLinkDeleteHandler)
	})

	fmt.Printf("Platform API server starting on %s\n", config.ServerPort)
	fmt.Printf("MongoDB: %s/%s\n", config.MongoURI, config.MongoDB)
	log.Fatal(http.ListenAndServe(config.ServerPort, r))
}
