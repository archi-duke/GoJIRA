package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"gojira-api/config"
	"gojira-api/db"
	"gojira-api/handlers"
)

func caseInsensitiveRouting(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(r.URL.Path, "/", 5)
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
			r.URL.Path = strings.Join(parts, "/")
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	db.Init()

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
		// Check
		r.Get("/Check", handlers.CheckHandler)

		// Issue CRUD
		r.Post("/Issue", handlers.IssueCreateHandler)
		r.Get("/Issue", handlers.IssueGetHandler)
		r.Put("/Issue", handlers.IssueUpdateHandler)
		r.Delete("/Issue", handlers.IssueDeleteHandler)

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

		// 문서 생성/배포 요청
		r.Post("/Issue/docgen", handlers.DocGenHandler)

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
		r.Get("/Project/alerts", handlers.ProjectAlertsHandler)
	})

	fmt.Printf("GoJIRA API server starting on %s\n", config.ServerPort)
	fmt.Printf("MongoDB: %s/%s\n", config.MongoURI, config.MongoDB)
	fmt.Printf("JIRA Mock: %s\n", config.JiraMockURL)
	log.Fatal(http.ListenAndServe(config.ServerPort, r))
}
