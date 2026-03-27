package handlers

import (
	"encoding/json"
	"net/http"
)

func CheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode("GoJIRA API is running")
}
