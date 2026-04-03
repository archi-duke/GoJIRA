package models

import "time"

// Notice 공지사항
type Notice struct {
	ID          string    `json:"id" bson:"_id,omitempty"`
	Title       string    `json:"title" bson:"title"`
	Body        string    `json:"body" bson:"body"`           // 마크다운 또는 HTML
	BodyType    string    `json:"bodyType" bson:"bodyType"`   // "markdown" | "html"
	Status      string    `json:"status" bson:"status"`       // "draft" | "active" | "expired"
	IsEmergency bool      `json:"isEmergency" bson:"isEmergency"`
	StartDate   time.Time `json:"startDate" bson:"startDate"`
	EndDate     time.Time `json:"endDate" bson:"endDate"`
	Views       int       `json:"views" bson:"views"`
	IsActive    bool      `json:"isActive" bson:"isActive"`
	CreatedAt   time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" bson:"updatedAt"`
	CreatedBy   string    `json:"createdBy" bson:"createdBy"`
	UpdatedBy   string    `json:"updatedBy" bson:"updatedBy"`
}
