package cmd

import (
	"fmt"
	"net/url"
)

func PR(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: gojira-cli pr <create|list> [options]")
		return
	}
	switch args[0] {
	case "create":
		prCreate(args[1:])
	case "list":
		prList(args[1:])
	default:
		fmt.Printf("Unknown subcommand: pr %s\n", args[0])
	}
}

func prCreate(args []string) {
	project := parseFlag(args, "project")
	title := parseFlag(args, "title")
	body := parseFlag(args, "body")
	if project == "" {
		fatal("--project is required")
	}
	if title == "" {
		title = fmt.Sprintf("[%s] 설계문서 배포 요청", project)
	}
	user := userID()
	result, err := platformPost(fmt.Sprintf("/DocRepo/%s/pr", project), map[string]string{
		"title": title, "body": body, "head": "docs/" + user, "base": "main",
	})
	if err != nil {
		fatal(err.Error())
	}
	outputJSON(result)
}

func prList(args []string) {
	project := parseFlag(args, "project")
	state := parseFlag(args, "state")
	if project == "" {
		fatal("--project is required")
	}
	if state == "" {
		state = "open"
	}
	result, err := platformGet(fmt.Sprintf("/DocRepo/%s/prs?state=%s", project, url.QueryEscape(state)))
	if err != nil {
		fatal(err.Error())
	}
	outputJSON(result)
}
