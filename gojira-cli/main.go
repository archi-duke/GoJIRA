package main

import (
	"fmt"
	"os"

	"gojira-cli/cmd"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "project":
		cmd.Project(os.Args[2:])
	case "issue":
		cmd.Issue(os.Args[2:])
	case "doc":
		cmd.Doc(os.Args[2:])
	case "pr":
		cmd.PR(os.Args[2:])
	case "schedule":
		cmd.Schedule(os.Args[2:])
	case "satam":
		cmd.SATAM(os.Args[2:])
	case "analyze":
		cmd.Analyze(os.Args[2:])
	case "version":
		fmt.Printf("gojira-cli v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`gojira-cli — GoJIRA 프로젝트 관리 CLI

Usage:
  gojira-cli <command> <subcommand> [options]

Commands:
  project   list                          프로젝트 목록 조회
  issue     list|create|update|delete|link 이슈 관리
  doc       list|summarize|diff           문서 관리
  pr        create|list                   PR 관리
  schedule  analyze                       일정 분석
  version                                 버전 정보

Environment:
  GOJIRA_API_URL     GoJIRA API URL (default: http://localhost:9001/api/v2)
  GOJIRA_PLATFORM_URL Platform API URL (default: http://localhost:9000/api/v2)
  GOJIRA_USER        사용자 ID

Examples:
  gojira-cli project list
  gojira-cli issue list --project GOJIRA
  gojira-cli issue create --project GOJIRA --type VOS --summary "시스템 목표"
  gojira-cli doc summarize --project GOJIRA --file srs.md
  gojira-cli pr create --project GOJIRA`)
}
