package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Skill 정의
type Skill struct {
	ID          string                                                                          `json:"id"`
	Name        string                                                                          `json:"name"`
	Description string                                                                          `json:"description"`
	Parameters  json.RawMessage                                                                 `json:"parameters"` // JSON Schema
	Execute     func(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) `json:"-"`
}

// MCP Tool 형식 변환
func (s *Skill) ToTool() map[string]interface{} {
	var schema interface{}
	json.Unmarshal(s.Parameters, &schema)
	return map[string]interface{}{
		"name":         s.ID,
		"description":  s.Description,
		"input_schema": schema,
	}
}

// SkillRegistry Skill 저장소
type SkillRegistry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
	order  []string // 등록 순서 유지
}

var Default = &SkillRegistry{
	skills: make(map[string]*Skill),
}

func (r *SkillRegistry) Register(skill *Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[skill.ID]; !exists {
		r.order = append(r.order, skill.ID)
	}
	r.skills[skill.ID] = skill
}

func (r *SkillRegistry) Get(id string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skills[id]
}

func (r *SkillRegistry) Execute(ctx context.Context, id string, params map[string]interface{}, user string) (interface{}, error) {
	skill := r.Get(id)
	if skill == nil {
		return nil, fmt.Errorf("skill not found: %s", id)
	}
	return skill.Execute(ctx, params, user)
}

// ListTools MCP tools 형식으로 전체 Skill 목록 반환
func (r *SkillRegistry) ListTools() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]map[string]interface{}, 0, len(r.order))
	for _, id := range r.order {
		tools = append(tools, r.skills[id].ToTool())
	}
	return tools
}

// Register 전역 등록
func Register(skill *Skill) { Default.Register(skill) }

// MustJSON JSON Schema 헬퍼
func MustJSON(s string) json.RawMessage {
	var v json.RawMessage
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(fmt.Sprintf("invalid JSON schema: %v", err))
	}
	return v
}
