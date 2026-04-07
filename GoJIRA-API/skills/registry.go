package skills

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"
)

// SkillDef SKILL.md에서 파싱된 Skill 정의
type SkillDef struct {
	Name        string                       `yaml:"name"`
	Description string                       `yaml:"description"`
	Parameters  map[string]ParameterDef      `yaml:"parameters"`
	API         *APIDef                      `yaml:"api"`
	Actions     map[string]APIDef            `yaml:"actions"`
	Prompt      string                       `yaml:"-"` // frontmatter 이후 마크다운 본문
	Dir         string                       `yaml:"-"` // SKILL.md가 위치한 디렉토리
	LLMPost     bool                         `yaml:"llm_post_process"`
	PromptTmpl  string                       `yaml:"prompt_template"`
	DataSources []map[string]interface{}     `yaml:"data_sources"`
	Protected   []string                     `yaml:"protected_files"`
}

type ParameterDef struct {
	Type        string   `yaml:"type"`
	Description string   `yaml:"description"`
	Required    bool     `yaml:"required"`
	Enum        []string `yaml:"enum"`
}

type APIDef struct {
	Method       string            `yaml:"method"`
	Path         string            `yaml:"path"`
	Paths        []string          `yaml:"paths"`
	Headers      map[string]string `yaml:"headers"`
	PlatformAPI  bool              `yaml:"platform_api"`
	AutoKey      bool              `yaml:"auto_key"`
	Bidirectional bool             `yaml:"bidirectional"`
	LLMPost      bool              `yaml:"llm_post_process"`
	PromptTmpl   string            `yaml:"prompt_template"`
	BodyTemplate map[string]string `yaml:"body_template"`
	Defaults     map[string]string `yaml:"defaults"`
	ResponseFields []string        `yaml:"response_fields"`
	ResponseFilter map[string]string `yaml:"response_filter"`
}

// SkillRegistry Skill 저장소
type SkillRegistry struct {
	mu     sync.RWMutex
	skills map[string]*SkillDef
	order  []string
}

var Default = &SkillRegistry{
	skills: make(map[string]*SkillDef),
}

func (r *SkillRegistry) Register(skill *SkillDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[skill.Name]; !exists {
		r.order = append(r.order, skill.Name)
	}
	r.skills[skill.Name] = skill
}

func (r *SkillRegistry) Get(name string) *SkillDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skills[name]
}

func (r *SkillRegistry) List() []*SkillDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*SkillDef, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.skills[name])
	}
	return result
}

// ListTools MCP tools 형식으로 반환
func (r *SkillRegistry) ListTools() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]map[string]interface{}, 0, len(r.order))
	for _, name := range r.order {
		s := r.skills[name]
		tools = append(tools, s.ToTool())
	}
	return tools
}

// ToTool MCP/OpenAI tool 형식 변환
func (s *SkillDef) ToTool() map[string]interface{} {
	props := map[string]interface{}{}
	var required []string
	for k, p := range s.Parameters {
		prop := map[string]interface{}{
			"type":        p.Type,
			"description": p.Description,
		}
		if len(p.Enum) > 0 {
			prop["enum"] = p.Enum
		}
		props[k] = prop
		if p.Required {
			required = append(required, k)
		}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return map[string]interface{}{
		"name":         s.Name,
		"description":  s.Description,
		"input_schema": schema,
	}
}

// LoadSkillsFromDir 디렉토리에서 SKILL.md 파일을 찾아 로드
func LoadSkillsFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("skills dir read failed: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		skill, err := ParseSkillFile(skillFile)
		if err != nil {
			log.Printf("Skill 로드 실패 (%s): %v", skillFile, err)
			continue
		}
		skill.Dir = filepath.Join(dir, entry.Name())
		Default.Register(skill)
		log.Printf("Skill 로드: %s (%s)", skill.Name, skill.Description)
	}
	return nil
}

// ParseSkillFile SKILL.md 파일 파싱 (YAML frontmatter + markdown body)
func ParseSkillFile(path string) (*SkillDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)

	// frontmatter 분리
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("frontmatter parse failed: %w", err)
	}

	var skill SkillDef
	if err := yaml.Unmarshal([]byte(frontmatter), &skill); err != nil {
		return nil, fmt.Errorf("YAML parse failed: %w", err)
	}
	skill.Prompt = body

	if skill.Name == "" {
		skill.Name = filepath.Base(filepath.Dir(path))
	}

	return &skill, nil
}

func splitFrontmatter(content string) (string, string, error) {
	if len(content) < 3 || content[:3] != "---" {
		return "", content, nil
	}
	// 두 번째 --- 찾기
	end := -1
	for i := 3; i < len(content); i++ {
		if i+3 <= len(content) && content[i:i+3] == "---" && (i == 3 || content[i-1] == '\n') {
			end = i
			break
		}
	}
	if end == -1 {
		return "", content, fmt.Errorf("closing --- not found")
	}
	fm := content[3:end]
	body := ""
	if end+3 < len(content) {
		body = content[end+3:]
	}
	return fm, body, nil
}

// AutoLoadSkills 기본 경로에서 Skill 자동 로드
func AutoLoadSkills() {
	// 1. 환경변수
	if d := os.Getenv("GOJIRA_SKILLS_DIR"); d != "" {
		LoadSkillsFromDir(d)
		return
	}
	// 2. 소스 파일 기준
	if _, filename, _, ok := runtime.Caller(0); ok {
		dir := filepath.Dir(filename)
		if err := LoadSkillsFromDir(dir); err == nil && len(Default.order) > 0 {
			return
		}
	}
	// 3. 현재 디렉토리 기준
	LoadSkillsFromDir("skills")
}

// MustJSON JSON 문자열을 RawMessage로 변환 (하위 호환)
func MustJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}
