package models

import "time"

// User 사용자 정보
type User struct {
	ID           string    `json:"id" bson:"_id,omitempty"`
	KnoxID       string    `json:"knoxId" bson:"knoxId"`
	Name         string    `json:"name" bson:"name"`
	Email        string    `json:"email" bson:"email"`
	AuthGroupIDs []string  `json:"authGroupIds" bson:"authGroupIds"`
	OrgUnitIDs   []string  `json:"orgUnitIds" bson:"orgUnitIds"`
	IsActive     bool      `json:"isActive" bson:"isActive"`
	CreatedAt    time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt" bson:"updatedAt"`
	CreatedBy    string    `json:"createdBy" bson:"createdBy"`
	UpdatedBy    string    `json:"updatedBy" bson:"updatedBy"`
}

// AuthGroup 권한 그룹
type AuthGroup struct {
	ID          string         `json:"id" bson:"_id,omitempty"`
	Code        string         `json:"code" bson:"code"`
	Name        string         `json:"name" bson:"name"`
	Description string         `json:"description" bson:"description"`
	Authority   map[string]int `json:"authority" bson:"authority"`
	IsActive    bool           `json:"isActive" bson:"isActive"`
	CreatedAt   time.Time      `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt" bson:"updatedAt"`
	CreatedBy   string         `json:"createdBy" bson:"createdBy"`
	UpdatedBy   string         `json:"updatedBy" bson:"updatedBy"`
}

// OrgUnit 조직 단위
type OrgUnit struct {
	ID              string    `json:"id" bson:"_id,omitempty"`
	Name            string    `json:"name" bson:"name"`
	Type            string    `json:"type" bson:"type"`
	ParentOrgUnitID string    `json:"parentOrgUnitId" bson:"parentOrgUnitId"`
	SortOrder       int       `json:"sortOrder" bson:"sortOrder"`
	IsActive        bool      `json:"isActive" bson:"isActive"`
	CreatedAt       time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt" bson:"updatedAt"`
	CreatedBy       string    `json:"createdBy" bson:"createdBy"`
	UpdatedBy       string    `json:"updatedBy" bson:"updatedBy"`
}

// UserProject 사용자-프로젝트 참여
type UserProject struct {
	ID        string     `json:"id" bson:"_id,omitempty"`
	UserID    string     `json:"userId" bson:"userId"`
	ProjectID string     `json:"projectId" bson:"projectId"`
	Role      string     `json:"role" bson:"role"`
	IsActive  bool       `json:"isActive" bson:"isActive"`
	JoinedAt  time.Time  `json:"joinedAt" bson:"joinedAt"`
	LeftAt    *time.Time `json:"leftAt" bson:"leftAt"`
	CreatedAt time.Time  `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt" bson:"updatedAt"`
	CreatedBy string     `json:"createdBy" bson:"createdBy"`
	UpdatedBy string     `json:"updatedBy" bson:"updatedBy"`
}

// UserConnection 접속 상태
type UserConnection struct {
	ID             string     `json:"id" bson:"_id,omitempty"`
	UserID         string     `json:"userId" bson:"userId"`
	ConnectionID   string     `json:"connectionId" bson:"connectionId"`
	ClientType     string     `json:"clientType" bson:"clientType"`
	IsOnline       bool       `json:"isOnline" bson:"isOnline"`
	ConnectedAt    time.Time  `json:"connectedAt" bson:"connectedAt"`
	LastSeenAt     time.Time  `json:"lastSeenAt" bson:"lastSeenAt"`
	DisconnectedAt *time.Time `json:"disconnectedAt" bson:"disconnectedAt"`
	CreatedAt      time.Time  `json:"createdAt" bson:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt" bson:"updatedAt"`
}

// 권한 비트마스크 상수
const (
	PermCreate = 8 // C
	PermRead   = 4 // R
	PermUpdate = 2 // U
	PermDelete = 1 // D
)
