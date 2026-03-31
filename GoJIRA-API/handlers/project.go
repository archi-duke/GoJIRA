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
	code := req.MainJiraCode()
	if req.Name == "" || code == "" {
		respondError(w, http.StatusBadRequest, "name and jiraCodes.main.jiraCode are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	doc := bson.M{
		"name":       req.Name,
		"jiraCodes":  bson.M{"main": bson.M{"jiraCode": code}},
		"status":     req.Status,
		"devTeam":    req.DevTeam,
		"devGroup":   req.DevGroup,
		"members":    req.Members,
		"feature":    req.Feature,
		"architect":  req.Architect,
		"PL":         req.PL,
		"schedules":  req.Schedules,
		"issueQuery": req.IssueQuery,
		"PLMCode":    req.PLMCode,
		"diagrams":   req.Diagrams,
		"documents":  req.Documents,
		"createdAt":  time.Now(),
	}

	if err := db.UpsertProject(ctx, code, doc); err != nil {
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
		filter = bson.M{"jiraCodes.main.jiraCode": req.JiraCode}
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
	code := req.MainJiraCode()
	if code == "" {
		respondError(w, http.StatusBadRequest, "jiraCodes.main.jiraCode is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	doc := bson.M{
		"name":       req.Name,
		"jiraCodes":  bson.M{"main": bson.M{"jiraCode": code}},
		"status":     req.Status,
		"devTeam":    req.DevTeam,
		"devGroup":   req.DevGroup,
		"members":    req.Members,
		"feature":    req.Feature,
		"architect":  req.Architect,
		"PL":         req.PL,
		"schedules":  req.Schedules,
		"issueQuery": req.IssueQuery,
		"PLMCode":    req.PLMCode,
		"diagrams":   req.Diagrams,
		"documents":  req.Documents,
	}

	if err := db.UpsertProject(ctx, code, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	projects, _ := db.FindAllProjects(ctx, nil)
	respondJSON(w, http.StatusOK, projects)
}
