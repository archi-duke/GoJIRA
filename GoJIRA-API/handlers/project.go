package handlers

import (
	"context"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
	"gojira-api/models"
)

// POST /api/v{version}/Project
func ProjectCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.ProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.JiraCode == "" {
		respondError(w, http.StatusBadRequest, "Name and JiraCode are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	doc := bson.M{
		"Name":      req.Name,
		"JiraCode":  req.JiraCode,
		"State":     req.State,
		"DevTeam":   req.DevTeam,
		"DevGroup":  req.DevGroup,
		"Members":   req.Members,
		"Feature":   req.Feature,
		"Architect": req.Architect,
		"PL":        req.PL,
		"Schedules": req.Schedules,
		"Filter":    req.Filter,
		"PLMCode":   req.PLMCode,
		"Diagrams":  req.Diagrams,
		"Documents": req.Documents,
		"createdAt": time.Now(),
	}

	if err := db.UpsertProject(ctx, req.JiraCode, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	projects, _ := db.FindAllProjects(ctx, nil)
	respondJSON(w, http.StatusOK, projects)
}

// GET /api/v{version}/Project
func ProjectGetHandler(w http.ResponseWriter, r *http.Request) {
	var req models.ProjectGetRequest
	parseXBody(r, &req)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var filter bson.M
	if req.JiraCode != "" {
		filter = bson.M{"JiraCode": req.JiraCode}
	}

	projects, err := db.FindAllProjects(ctx, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, projects)
}

// PUT /api/v{version}/Project
func ProjectUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.ProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.JiraCode == "" {
		respondError(w, http.StatusBadRequest, "JiraCode is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	doc := bson.M{
		"Name":      req.Name,
		"JiraCode":  req.JiraCode,
		"State":     req.State,
		"DevTeam":   req.DevTeam,
		"DevGroup":  req.DevGroup,
		"Members":   req.Members,
		"Feature":   req.Feature,
		"Architect": req.Architect,
		"PL":        req.PL,
		"Schedules": req.Schedules,
		"Filter":    req.Filter,
		"PLMCode":   req.PLMCode,
		"Diagrams":  req.Diagrams,
		"Documents": req.Documents,
	}

	if err := db.UpsertProject(ctx, req.JiraCode, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	projects, _ := db.FindAllProjects(ctx, nil)
	respondJSON(w, http.StatusOK, projects)
}
