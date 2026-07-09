package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistInstallAnswers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	// git 크리덴셜이 이미 채워진 기존 launcher.json
	orig := `{
  "defaultAgent": "claude",
  "port": 5599,
  "git": { "base": "http://gitea:3300", "org": "gojira", "user": "duke", "token": "SECRET-PAT" }
}
`
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := persistInstallAnswers(dir, "https://gojira-host:44300", "", ""); err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["gateway"] != "https://gojira-host:44300" {
		t.Errorf("gateway 미기록: %v", m["gateway"])
	}
	// git 크리덴셜 보존 확인
	git, _ := m["git"].(map[string]any)
	if git == nil || git["token"] != "SECRET-PAT" || git["user"] != "duke" {
		t.Errorf("git 크리덴셜 유실: %v", m["git"])
	}
	if m["defaultAgent"] != "claude" {
		t.Errorf("기타 필드 유실: defaultAgent=%v", m["defaultAgent"])
	}

	// 파일이 없으면 placeholder 생성 후 기록
	dir2 := t.TempDir()
	if err := persistInstallAnswers(dir2, "http://h:3000", "", ""); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(filepath.Join(dir2, configFileName))
	m2 := map[string]any{}
	json.Unmarshal(data2, &m2)
	if m2["gateway"] != "http://h:3000" {
		t.Errorf("신규 파일 gateway 미기록: %v", m2["gateway"])
	}
}

func TestGatewayOrigin(t *testing.T) {
	cases := map[string]string{
		"https://gojira-host:44300":              "https://gojira-host:44300",
		"http://10.0.0.1:3000/":                  "http://10.0.0.1:3000",
		"https://g.corp:443/platform/api/v2": "https://g.corp:443",
		"  https://g.corp  ":                     "https://g.corp",
		"gojira-host:44300":                      "", // scheme 없음 → 무효
		"":                                       "",
	}
	for in, want := range cases {
		if got := gatewayOrigin(in); got != want {
			t.Errorf("gatewayOrigin(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDeriveGateway(t *testing.T) {
	// gateway 만 있을 때 backend + webOrigins 파생
	c := &Config{Gateway: "https://gojira-host:44300"}
	deriveGateway(c)
	if c.Backend.PlatformAPI != "https://gojira-host:44300/platform/api/v2" {
		t.Errorf("platformApi=%q", c.Backend.PlatformAPI)
	}
	if c.Backend.GojiraAPI != "https://gojira-host:44300/gojira/api/v2" {
		t.Errorf("gojiraApi=%q", c.Backend.GojiraAPI)
	}
	if len(c.WebOrigins) != 1 || c.WebOrigins[0] != "https://gojira-host:44300" {
		t.Errorf("webOrigins=%v", c.WebOrigins)
	}

	// backend 수동 override 는 존중, webOrigins 중복 추가 안 함
	c2 := &Config{
		Gateway:    "https://g:443/",
		Backend:    Backend{PlatformAPI: "https://manual/api"},
		WebOrigins: []string{"https://g:443"},
	}
	deriveGateway(c2)
	if c2.Backend.PlatformAPI != "https://manual/api" {
		t.Errorf("override 무시됨: %q", c2.Backend.PlatformAPI)
	}
	if c2.Backend.GojiraAPI != "https://g:443/gojira/api/v2" {
		t.Errorf("gojiraApi=%q", c2.Backend.GojiraAPI)
	}
	if len(c2.WebOrigins) != 1 {
		t.Errorf("중복 origin 추가됨: %v", c2.WebOrigins)
	}

	// gateway 없으면 no-op
	c3 := &Config{}
	deriveGateway(c3)
	if c3.Backend.PlatformAPI != "" || len(c3.WebOrigins) != 0 {
		t.Errorf("gateway 없는데 파생됨")
	}
}

func TestParseGatewayFlag(t *testing.T) {
	if g := parseGatewayFlag([]string{"--gateway", "https://x:1"}); g != "https://x:1" {
		t.Errorf("space form=%q", g)
	}
	if g := parseGatewayFlag([]string{"--gateway=https://y:2"}); g != "https://y:2" {
		t.Errorf("eq form=%q", g)
	}
	if g := parseGatewayFlag([]string{"foo"}); g != "" {
		t.Errorf("none=%q", g)
	}
}
