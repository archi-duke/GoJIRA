package cmd

import "fmt"

func Analyze(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: gojira-cli analyze <relations|satam> --project CODE")
		return
	}
	switch args[0] {
	case "relations":
		analyzeRelations(args[1:])
	case "satam":
		satamAnalyze(args[1:])
	default:
		fmt.Printf("Unknown subcommand: analyze %s\n", args[0])
	}
}

func analyzeRelations(args []string) {
	project := parseFlag(args, "project")
	if project == "" {
		fatal("--project is required")
	}
	result, err := apiPost("/Agent/chat", map[string]interface{}{
		"message": fmt.Sprintf("%s 프로젝트의 이슈 관계를 분석해줘", project),
		"context": map[string]string{"page": "relationMap", "projectCode": project},
	})
	if err != nil {
		fatal(err.Error())
	}
	rm, _ := result.(map[string]interface{})
	fmt.Println(rm["reply"])
}
