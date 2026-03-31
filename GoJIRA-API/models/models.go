package models

// --- Request Models ---

type IssueCreateRequest struct {
	PrjID      string   `json:"prjID"`
	ProjectKey string   `json:"projectKey,omitempty"`
	IssueType  string   `json:"issueType"`
	ReqType    string   `json:"reqtype,omitempty"`
	Summary    string   `json:"summary"`
	Desc       string   `json:"desc,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	SubKey     string   `json:"subKey,omitempty"`
}

type IssueGetRequest struct {
	PrjID    string `json:"prjID"`
	InfoType string `json:"infoType"`
	Query    string `json:"query,omitempty"`
}

type IssueUpdateRequest struct {
	PrjID string                 `json:"prjID"`
	Key   string                 `json:"key"`
	Data  map[string]interface{} `json:"data"`
}

type IssueDeleteRequest struct {
	PrjID string `json:"prjID"`
	Key   string `json:"key"`
}

type LabelRequest struct {
	PrjID        string `json:"prjID"`
	Key          string `json:"key"`
	LabelContent string `json:"labelContent"`
}

type LinkCreateRequest struct {
	PrjID      string `json:"prjID"`
	InwardKey  string `json:"inwardKey"`
	OutwardKey string `json:"outwardKey"`
	LinkType   string `json:"linkType,omitempty"`
}

type LinkDeleteRequest struct {
	PrjID  string `json:"prjID"`
	LinkID string `json:"linkId"`
}

type CommentRequest struct {
	PrjID     string `json:"prjID"`
	Key       string `json:"key"`
	CommentID string `json:"commentId,omitempty"`
	Body      string `json:"body"`
}

type CommentDeleteRequest struct {
	PrjID     string `json:"prjID"`
	Key       string `json:"key"`
	CommentID string `json:"commentId"`
}

type FetchRequest struct {
	PrjID     string `json:"prjID"`
	Query     string `json:"query,omitempty"`
	MergeType string `json:"mergeType,omitempty"`
}


type FieldUpdateRequest struct {
	PrjID string      `json:"prjID"`
	Key   string      `json:"key"`
	Field string      `json:"field"`
	Value interface{} `json:"value"`
}

type RelationsRequest struct {
	PrjID string `json:"prjID"`
	Query string `json:"query,omitempty"`
}

type AccountRequest struct {
	JiraID string `json:"jiraID"`
	Pswd   string `json:"pswd"`
}

type PageImagesRequest struct {
	PageID string        `json:"pageID"`
	Images []interface{} `json:"images"`
}

type JiraCodeEntry struct {
	JiraCode string `json:"jiraCode"`
}

type JiraCodes struct {
	Main JiraCodeEntry `json:"main"`
}

type ProjectRequest struct {
	Name       string                 `json:"name"`
	JiraCodes  JiraCodes              `json:"jiraCodes"`
	Schedules  []map[string]string    `json:"schedules,omitempty"`
	Feature    string                 `json:"feature,omitempty"`
	Architect  map[string]string      `json:"architect,omitempty"`
	PL         map[string]string      `json:"PL,omitempty"`
	DevTeam    string                 `json:"devTeam,omitempty"`
	DevGroup   string                 `json:"devGroup,omitempty"`
	Members    []string               `json:"members,omitempty"`
	Status     string                 `json:"status,omitempty"`
	IssueQuery string                 `json:"issueQuery,omitempty"`
	PLMCode    []string               `json:"PLMCode,omitempty"`
	Diagrams   map[string]interface{} `json:"diagrams,omitempty"`
	Documents  map[string]string      `json:"documents,omitempty"`
}

// MainJiraCode returns the main jira code string for convenience.
func (r *ProjectRequest) MainJiraCode() string {
	return r.JiraCodes.Main.JiraCode
}

type ProjectGetRequest struct {
	JiraCode string `json:"jiraCode,omitempty"`
}
