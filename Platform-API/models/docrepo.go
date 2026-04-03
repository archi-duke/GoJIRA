package models

import "time"

// DocRepo 프로젝트-GitHub repo 매핑
type DocRepo struct {
	ID            string    `json:"id" bson:"_id,omitempty"`
	ProjectID     string    `json:"projectId" bson:"projectId"`
	RepoName      string    `json:"repoName" bson:"repoName"`
	RepoOwner     string    `json:"repoOwner" bson:"repoOwner"`
	RepoURL       string    `json:"repoUrl" bson:"repoUrl"`
	DefaultBranch string    `json:"defaultBranch" bson:"defaultBranch"`
	Auditors      []string  `json:"auditors" bson:"auditors"`
	IsActive      bool      `json:"isActive" bson:"isActive"`
	CreatedAt     time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt" bson:"updatedAt"`
	CreatedBy     string    `json:"createdBy" bson:"createdBy"`
	UpdatedBy     string    `json:"updatedBy" bson:"updatedBy"`
}

// DocLink 문서-코드 연결
type DocLink struct {
	ID          string    `json:"id" bson:"_id,omitempty"`
	TaskID      string    `json:"taskId" bson:"taskId"`
	DocRepoID   string    `json:"docRepoId" bson:"docRepoId"`
	CodeRepoURL string    `json:"codeRepoUrl" bson:"codeRepoUrl"`
	CodePath    string    `json:"codePath" bson:"codePath"`
	PRUrl       string    `json:"prUrl" bson:"prUrl"`
	IsActive    bool      `json:"isActive" bson:"isActive"`
	CreatedAt   time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" bson:"updatedAt"`
	CreatedBy   string    `json:"createdBy" bson:"createdBy"`
	UpdatedBy   string    `json:"updatedBy" bson:"updatedBy"`
}
