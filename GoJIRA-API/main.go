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

// caseInsensitiveRouting normalizes URL path to match registered routes.
func caseInsensitiveRouting(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Preserve version prefix, normalize the rest
		parts := strings.SplitN(r.URL.Path, "/", 5) // ["", "api", "v1", "rest..."]
		if len(parts) >= 5 {
			// Rebuild with original casing for path params but normalize route segments
			// Map known routes to their canonical casing
			canonical := map[string]string{
				"check":   "Check",
				"issue":   "Issue",
				"project": "Project",
				"jira":    "Jira",
				"page":    "Page",
			}
			seg := parts[3]
			if mapped, ok := canonical[strings.ToLower(seg)]; ok {
				parts[3] = mapped
			}
			r.URL.Path = strings.Join(parts, "/")
		} else if len(parts) == 4 {
			canonical := map[string]string{
				"check":   "Check",
				"issue":   "Issue",
				"project": "Project",
				"jira":    "Jira",
				"page":    "Page",
			}
			seg := parts[3]
			if mapped, ok := canonical[strings.ToLower(seg)]; ok {
				parts[3] = mapped
			}
			r.URL.Path = strings.Join(parts, "/")
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	// Initialize MongoDB
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

		// Field
		r.Put("/Issue/field", handlers.FieldUpdateHandler)

		// Relations
		r.Put("/Issue/relations", handlers.RelationsHandler)

		// Account
		r.Post("/Jira/Account", handlers.AccountCreateHandler)

		// Page
		r.Post("/Page/images", handlers.PageImagesPostHandler)
		r.Put("/Page/images", handlers.PageImagesPutHandler)

		// Project
		r.Post("/Project", handlers.ProjectCreateHandler)
		r.Get("/Project", handlers.ProjectGetHandler)
		r.Put("/Project", handlers.ProjectUpdateHandler)
	})

	fmt.Printf("GoJIRA API server starting on %s\n", config.ServerPort)
	fmt.Printf("MongoDB: %s/%s\n", config.MongoURI, config.MongoDB)
	fmt.Printf("JIRA Mock: %s\n", config.JiraMockURL)
	log.Fatal(http.ListenAndServe(config.ServerPort, r))
}
