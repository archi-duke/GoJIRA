package main

import (
	"fmt"
	"log"
	"net/http"

	"conf-mock/handlers"
)

func main() {
	port := 8090

	mux := http.NewServeMux()

	mux.HandleFunc("/rest/api/content/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.GetContent(w, r)
		case http.MethodPut:
			handlers.UpdateContent(w, r)
		case http.MethodDelete:
			handlers.DeleteContent(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/rest/api/content", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handlers.CreateContent(w, r)
		case http.MethodGet:
			handlers.SearchContent(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Health check
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"state":"RUNNING"}`))
	})

	log.Printf("Confluence DC Mock Server listening on :%d", port)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST   /rest/api/content          - 페이지 생성\n")
	fmt.Printf("  GET    /rest/api/content           - 목록/검색\n")
	fmt.Printf("  GET    /rest/api/content/{id}      - 단건 조회\n")
	fmt.Printf("  PUT    /rest/api/content/{id}      - 페이지 수정\n")
	fmt.Printf("  DELETE /rest/api/content/{id}      - 페이지 삭제\n")
	fmt.Printf("  GET    /status                     - 상태 확인\n")

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), mux); err != nil {
		log.Fatal(err)
	}
}
