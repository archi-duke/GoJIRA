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
	SubKey     string   `json:"subkey,omitempty"`
	User       string   `json:"user,omitempty"`
	// LocalOnly — true 면 Mongo 캐시 upsert(전역 조율 불필요 단계)를 건너뛴다.
	// key/subkey 할당과 project 존재 확인은 여전히 수행한다(전역 조율 필수).
	// (web-local-view-model.md §Phase 2)
	LocalOnly bool `json:"localOnly,omitempty"`
}

type IssueGetRequest struct {
	PrjID    string `json:"prjID"`
	InfoType string `json:"infoType"`
	Query    string `json:"query,omitempty"`
	User     string `json:"user,omitempty"`
}

type IssueUpdateRequest struct {
	PrjID string                 `json:"prjID"`
	Key   string                 `json:"key"`
	Data  map[string]interface{} `json:"data"`
	User  string                 `json:"user,omitempty"`
	// BaseYaml — 클라이언트가 동봉한 로컬 YAML(진실). 비어있지 않으면 Mongo 무접촉
	// 로컬 병합 경로를 탄다 (web-local-view-model.md §Phase 2). 비어있으면 기존
	// Mongo 기반 경로(전환기 호환).
	BaseYaml string `json:"baseYaml,omitempty"`
}

type IssueDeleteRequest struct {
	PrjID string `json:"prjID"`
	Key   string `json:"key"`
	User  string `json:"user,omitempty"`
	// BaseYaml — 있으면 로컬 삭제 경로(Mongo 무접촉): subkey 추출 후 delete:true
	// localWrite 만 반환한다. 공식본 삭제는 머지 reconcile 이 수행.
	BaseYaml string `json:"baseYaml,omitempty"`
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
	// Source — 호출 SKILL 이름 (예: "requirement-refine2"). drift-check 가 이 필드 있는 link 를
	// "정상 워크플로우 자동 생성" 으로 판단해 hash 에서 제외. 사용자 수작업 link 는 source 비어있음.
	Source string `json:"source,omitempty"`
	// InwardYaml / OutwardYaml — 클라이언트가 동봉한 로컬 YAML(진실). 둘 중 하나라도
	// 있으면 Mongo 무접촉 로컬 링크 추가 경로를 탄다 (§Phase 2). 한쪽만 있어도 되며
	// (타 과제 이슈가 로컬 clone 에 없을 수 있음), 둘 다 없으면 기존 Mongo 경로.
	InwardYaml  string `json:"inwardYaml,omitempty"`
	OutwardYaml string `json:"outwardYaml,omitempty"`
}

type LinkDeleteRequest struct {
	PrjID  string `json:"prjID"`
	LinkID string `json:"linkId"`
	// BaseFiles — 클라이언트가 동봉한 로컬 YAML(진실) 목록. 길이 > 0 이면 Mongo
	// 무접촉 로컬 링크 삭제 경로를 탄다 (§Phase 2).
	BaseFiles []string `json:"baseFiles,omitempty"`
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
	RepoName   string                 `json:"repoName,omitempty"` // 신규 생성 시 Gitea repo 이름 (사용자 입력, 필수)
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
	Auditors   []string               `json:"auditors,omitempty"`
}

// MainJiraCode returns the main jira code string for convenience.
func (r *ProjectRequest) MainJiraCode() string {
	return r.JiraCodes.Main.JiraCode
}

type ProjectGetRequest struct {
	JiraCode string `json:"jiraCode,omitempty"`
}
