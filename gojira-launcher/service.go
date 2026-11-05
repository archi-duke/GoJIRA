package main

// 서비스 계약 (설계서 §9.1, §7.1).
//
// 런처는 "배포 + 소환 + 라우팅" 만 하고, 기능은 로컬 서비스가 제공한다(D2).
// 각 서비스는 패키지 안 service.json 으로 자기를 기술하며, 런처는 **설치된 서비스의
// 선언을 모아 라우팅 표**를 만든다 — 그래서 서비스를 추가해도 런처를 고칠 필요가 없다(D3).
//
// 1단계에서는 기존 런처 기능(①~④)을 "내장 서비스" 로 같은 표 뒤에 두어 동작을 보존한다.
// 3단계에서 gojira-workspace 외부 서비스로 떼어낼 때 이 표의 항목만 교체하면 된다.

import "strings"

// Route — 서비스가 담당하는 경로 선언.
type Route struct {
	Prefix     string   `json:"prefix"`               // 예: /v1/docs/pptx — 끝에 / 없음
	Methods    []string `json:"methods"`              // 비면 전체 허용
	TimeoutSec int      `json:"timeoutSec,omitempty"` // 중계 타임아웃(§7.3)
	MaxBodyMB  int      `json:"maxBodyMB,omitempty"`
}

// ServiceSpec — 패키지 내 service.json. 외부 서비스가 자기를 기술하는 형식이며,
// 내장 서비스도 같은 구조로 선언해 라우팅 표를 한 종류로 유지한다.
type ServiceSpec struct {
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Entry       string      `json:"entry,omitempty"`
	OS          []string    `json:"os,omitempty"`
	MinLauncher string      `json:"minLauncher,omitempty"`
	Run         RunSpec     `json:"run,omitempty"`
	Health      string      `json:"health,omitempty"`
	Requires    []string    `json:"requires,omitempty"`
	Permissions Permissions `json:"permissions,omitempty"`
	Routes      []Route     `json:"routes"`

	// builtin — 런처 안에서 직접 처리하는 내장 서비스 표시. true 면 Supervisor·Installer
	// 대상이 아니며 프로세스도 뜨지 않는다(1단계 한정, 3단계에서 제거 예정).
	builtin bool
}

type RunSpec struct {
	Mode           string `json:"mode,omitempty"` // always | on-demand
	IdleTimeoutMin int    `json:"idleTimeoutMin,omitempty"`
}

type Permissions struct {
	Spawn []string `json:"spawn,omitempty"`
	FS    []string `json:"fs,omitempty"`
	Net   []string `json:"net,omitempty"`
	Call  []string `json:"call,omitempty"` // 서비스 간 호출 허용 prefix (D10, §7.4)
}

// allowsMethod — 선언한 메서드에 해당하는지. Methods 가 비면 전체 허용.
func (rt Route) allowsMethod(m string) bool {
	if len(rt.Methods) == 0 {
		return true
	}
	for _, x := range rt.Methods {
		if strings.EqualFold(x, m) {
			return true
		}
	}
	return false
}

// matches — 세그먼트 경계 기준 prefix 일치(§7.2).
// `/v1/docs/pptx` 는 `/v1/docs/pptx`, `/v1/docs/pptx/text` 와 일치하고
// `/v1/docs/pptxx` 와는 일치하지 않는다.
func (rt Route) matches(path string) bool {
	return path == rt.Prefix || strings.HasPrefix(path, rt.Prefix+"/")
}

// builtinServices — 1단계 내장 서비스 선언 (§12.1 gojira-workspace 가 될 범위).
// 실제 처리는 기존 핸들러가 그대로 하고, 여기서는 "어떤 경로를 누가 갖는지" 만 선언한다.
// 외부 서비스가 같은 prefix 를 카탈로그에 등록하면 CI(build-catalog.sh)가 충돌로 막는다.
func builtinServices() []ServiceSpec {
	return []ServiceSpec{{
		Name:    "gojira-workspace",
		Version: version,
		Run:     RunSpec{Mode: "always"},
		builtin: true,
		Routes: []Route{
			{Prefix: "/v1/workspace", Methods: []string{"GET", "POST", "PUT", "DELETE"}},
			{Prefix: "/v1/agents", Methods: []string{"GET", "POST"}},
			{Prefix: "/v1/issues", Methods: []string{"GET", "POST"}},
		},
	}}
}
