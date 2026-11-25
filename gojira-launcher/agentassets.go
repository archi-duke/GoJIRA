package main

// agentAssetEntries — 런처가 각 프로젝트 clone 에 provision 하는 에이전트 자산의 최상위 엔트리.
// exe 에 embed(assets/) → skillsSourceDir 로 추출 → 각 clone 에 link/copy 되고,
// /sync(git add) · /files · /status 에서 제외된다.
//
//	.claude        — Claude Code / opencode 가 읽는 스킬 (.claude/skills/*/SKILL.md)
//	.opencode      — opencode 전용 에이전트 정의 (.opencode/agents/gojira.md)
//	opencode.json  — opencode 모델/권한 설정
//	AGENTS.md      — Codex 프로젝트 지시문 (.claude/CLAUDE.md·.claude/skills 로 안내)
//
// 에이전트(claude|opencode|codex)가 cwd=clone 루트에서 spawn 되므로(설계 §5-2), 이것들이 clone
// 안에 있어야 세 CLI 모두 스킬+설정을 인식한다. 과제 레포가 자기 AGENTS.md 를 갖고 있으면
// ensureAgentAssets 가 건드리지 않는다(이미 있으면 스킵).
var agentAssetEntries = []string{".claude", ".opencode", "opencode.json", "AGENTS.md"}

// isAgentAssetTop — 경로 최상위 세그먼트가 provision 자산인지.
func isAgentAssetTop(top string) bool {
	for _, e := range agentAssetEntries {
		if top == e {
			return true
		}
	}
	return false
}
