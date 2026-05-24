package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const version = "0.1.0"

// Server — 핸들러가 공유하는 설정 보관.
type Server struct {
	cfg *Config
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"status": "error", "error": msg})
}

// GET /health — 웹이 런처 설치/기동을 감지해 "수정" 버튼을 활성화 (§2.1).
//
// 토큰 핸드셰이크(설계 §6): Origin 이 webOrigin 과 일치하는 요청에만 응답에 token 을
// 포함한다. 브라우저는 Origin 을 위조할 수 없으므로 GoJIRA 웹만 토큰을 읽어 이후
// /edit·/open 호출에 사용한다. (DNS-rebinding 은 잔여 리스크 — §6)
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	agents := availableAgents()
	resp := map[string]any{
		"version": version,
		"agents":  agents,
		"ready":   hasGit() && hasEditor() && len(agents) > 0,
	}
	if s.originAllowed(r.Header.Get("Origin")) {
		resp["token"] = s.cfg.Token
	}
	writeJSON(w, http.StatusOK, resp)
}

type openReq struct {
	Code string `json:"code"`
	User string `json:"user"`
}

// POST /open — clone 보장 + docs/{user} 체크아웃 + VS Code 오픈 (§2.1).
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req openReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	repoPath, err := s.EnsureRepo(req.Code, req.User)
	if err != nil {
		log.Printf("/open 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 콜드스타트: 방금 내려받은 로컬 YAML → Mongo 캐시 동기화. YAML 이 진실이라 실패는 비치명적.
	ingest, err := s.IngestIssues(req.Code, req.User)
	if err != nil {
		log.Printf("/open ingest 경고 code=%s: %v", req.Code, err)
	}
	if err := OpenInEditor(repoPath, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "repoPath": repoPath, "ingest": ingest})
}

type editReq struct {
	Code string `json:"code"`
	User string `json:"user"`
	File string `json:"file"`
}

// POST /edit — clone 보장 + ingest + VS Code(특정 file) 오픈. agent spawn 은 /chat 으로 분리.
func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	var req editReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	repoPath, err := s.EnsureRepo(req.Code, req.User)
	if err != nil {
		log.Printf("/edit 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	fullFile, err := safeFilePath(repoPath, req.File)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 콜드스타트: 로컬 YAML → Mongo 캐시 동기화.
	ingest, err := s.IngestIssues(req.Code, req.User)
	if err != nil {
		log.Printf("/edit ingest 경고 code=%s: %v", req.Code, err)
	}
	if err := OpenInEditor(repoPath, fullFile); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"repoPath": repoPath,
		"ingest":   ingest,
	})
}

type chatReq struct {
	Code  string `json:"code"`
	User  string `json:"user"`
	Agent string `json:"agent"`
}

// POST /chat — 등록된 에이전트 CLI(opencode|claude) 만 새 터미널로 spawn.
// /edit 과 분리된 동작 — 사용자는 VS Code 만 열거나, 그 위에 채팅을 추가로 띄울 수 있다.
// code 가 비어있으면 프로젝트 없는 일반 채팅 — repoRoot 에서 그대로 spawn (clone 안 함).
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	agent, err := s.resolveAgent(req.Agent)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	spawnDir := s.cfg.RepoRoot
	if req.Code != "" {
		repoPath, err := s.EnsureRepo(req.Code, req.User)
		if err != nil {
			log.Printf("/chat 실패 code=%s user=%s: %v", req.Code, req.User, err)
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		spawnDir = repoPath
	}
	sessionID, err := SpawnAgent(agent, spawnDir, s.agentEnv(spawnDir, req.User))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"repoPath":  spawnDir,
		"agent":     agent,
		"sessionId": sessionID,
	})
}

type syncReq struct {
	Code    string `json:"code"`
	User    string `json:"user"`
	Message string `json:"message"` // 옵션 — 미지정시 auto-message.
}

// POST /sync — "등록" 액션: working tree dirty 면 stage+commit 한 뒤 origin 으로 push.
// 웹에서 미커밋(-) 또는 unpushed 가 하나라도 있으면 이 엔드포인트로 한 번에 해소한다.
// Push() 자체에는 손대지 않음 — 여기서 commit 만 보장하고 그 다음 기존 Push 흐름 재사용.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var req syncReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	path, err := s.EnsureRepo(req.Code, req.User)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	committed := false
	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		if _, err := gitCmd(path, "add", "-A"); err != nil {
			writeErr(w, http.StatusInternalServerError, "git add 실패: "+err.Error())
			return
		}
		msg := strings.TrimSpace(req.Message)
		if msg == "" {
			msg = "[GoJIRA-web] sync local changes"
		}
		if _, err := gitCmd(path, "commit", "-m", msg); err != nil {
			writeErr(w, http.StatusInternalServerError, "git commit 실패: "+err.Error())
			return
		}
		committed = true
	}
	res, err := s.Push(req.Code, req.User)
	if err != nil {
		log.Printf("/sync push 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"committed": committed,
		"push":      res,
	})
}

// POST /push — 로컬 docs/{user} 브랜치를 origin 으로 push (§5-3: push→PR 흐름).
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	var req openReq // {code, user}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	res, err := s.Push(req.Code, req.User)
	if err != nil {
		log.Printf("/push 실패 code=%s user=%s: %v", req.Code, req.User, err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type writeReq struct {
	Code    string `json:"code"`
	User    string `json:"user"`
	Path    string `json:"path"`
	Content string `json:"content"` // raw text (인라인 에디터 산출물). base64 인코딩 안 함.
}

// POST /write — 인라인 편집 저장. 로컬 working tree 의 단일 파일에 raw text 를 쓴다.
// commit/push 는 하지 않음 — Local 탭의 "등록" 흐름과 책임 분리(VS Code 저장과 동일 의미).
// safeFilePath 로 경로 컨테인먼트, 부모 디렉터리 자동 생성, mode 0644.
func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	var req writeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "잘못된 JSON 본문")
		return
	}
	if req.Path == "" {
		writeErr(w, http.StatusBadRequest, "path 누락")
		return
	}
	repoPath, err := s.EnsureRepo(req.Code, req.User)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	full, err := safeFilePath(repoPath, req.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(full, []byte(req.Content), 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"path":   req.Path,
		"size":   len(req.Content),
	})
}

// GET /files?code=&user= — 로컬 clone 의 working tree 파일 목록 + HEAD 트리.
// 응답 shape: {tree, headTree, total_count, source}.
//   - tree     : working tree (디스크에 있는 현재 내용 기준 blob sha) — 서버 /tree 와 호환.
//   - headTree : 로컬 docs/{user} HEAD 의 `git ls-tree -r HEAD` 결과 (커밋된 상태).
//
// 웹은 두 트리를 사용해 path 별로 "main vs local 차이" 외에 "이 차이가 커밋됐는가" 까지 판정해
// 배지에 +(committed) / -(uncommitted) 를 prefix 로 붙인다.
//
// sha 는 모두 git blob 해시라 비교 그대로 가능. .git / .claude(스킬 symlink) 는 노출 제외.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	user := r.URL.Query().Get("user")
	repoPath, err := s.EnsureRepo(code, user)
	if err != nil {
		log.Printf("/files 실패 code=%s user=%s: %v", code, user, err)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := listLocalTree(repoPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	headEntries, _ := listHeadTree(repoPath) // HEAD 가 없거나 ls-tree 실패면 빈 슬라이스로 진행(전부 uncommitted 로 표시됨).
	ahead := computeAhead(repoPath)          // upstream 대비 unpushed commit 수. -1 = upstream 미설정(=신규 브랜치, push 필요).
	writeJSON(w, http.StatusOK, map[string]any{
		"tree":        entries,
		"headTree":    headEntries,
		"ahead":       ahead,
		"total_count": len(entries),
		"source":      "local-launcher",
		"repoPath":    repoPath,
	})
}

// computeAhead — `@{upstream}..HEAD` commit 수. upstream 미설정이면 -1 (신규 브랜치 push 필요).
func computeAhead(repoPath string) int {
	out, err := gitCmd(repoPath, "rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		return -1
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

// GET /file?code=&user=&path= — 로컬 working tree 의 단일 파일 내용.
// 응답 shape 은 서버 /DocRepo/{code}/file 과 호환 — {path, content(base64), encoding:"base64", size, source}.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	user := r.URL.Query().Get("user")
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeErr(w, http.StatusBadRequest, "path 누락")
		return
	}
	repoPath, err := s.EnsureRepo(code, user)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	full, err := safeFilePath(repoPath, rel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			writeErr(w, http.StatusNotFound, "file 없음: "+rel)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     rel,
		"content":  base64.StdEncoding.EncodeToString(data),
		"encoding": "base64",
		"size":     len(data),
		"source":   "local-launcher",
	})
}

// listLocalTree — repoPath 하위 모든 파일을 git blob 해시와 함께 평면 리스트로 반환.
// .git / .claude(skills symlink) / 디렉터리 본체는 제외 (blob 만). 정렬은 path 사전순.
// 큰 파일이 많은 레포에서는 비용이 들 수 있으나 (수만 fork→0, sha1 read once),
// 설계문서 레포(수십~수백 파일) 기준 충분히 빠르다.
func listLocalTree(repoPath string) ([]map[string]any, error) {
	var entries []map[string]any
	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repoPath, path)
		if rel == "." {
			return nil
		}
		// 최상위 .git / .claude 컷오프 — 하위 traversal 도 스킵.
		top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		if top == ".git" || top == ".claude" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil // 한 파일 stat 실패는 무시 — 트리는 계속.
		}
		sha, err := blobSha(path, info.Size())
		if err != nil {
			return nil
		}
		entries = append(entries, map[string]any{
			"path": filepath.ToSlash(rel),
			"type": "blob",
			"sha":  sha,
			"size": info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i]["path"].(string) < entries[j]["path"].(string)
	})
	return entries, nil
}

// listHeadTree — `git ls-tree -r HEAD` 결과를 파싱해 평면 리스트로 반환.
// HEAD 가 없거나(빈 레포) 실패면 빈 리스트 + err (호출 측이 비치명적으로 처리).
// 포맷: <mode> <type> <sha>\t<path>
func listHeadTree(repoPath string) ([]map[string]any, error) {
	out, err := gitCmd(repoPath, "ls-tree", "-r", "HEAD")
	if err != nil {
		return []map[string]any{}, err
	}
	var entries []map[string]any
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		meta := strings.Fields(parts[0])
		if len(meta) < 3 {
			continue
		}
		entries = append(entries, map[string]any{
			"path": parts[1],
			"type": meta[1],
			"sha":  meta[2],
		})
	}
	return entries, nil
}

// blobSha — 파일 한 개의 git blob sha1 ("blob <size>\0<content>") 을 계산한다.
// 서버 main 트리의 sha 와 동일 포맷이라 그대로 비교(modified/unchanged 판단)에 사용 가능.
func blobSha(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", size)
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GET /status?code= — 로컬 작업/브랜치 진행 상태 (§2.1).
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	st, err := s.Status(code)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}
