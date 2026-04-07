package cmd

import "fmt"

func Schedule(args []string) {
	if len(args) == 0 || args[0] == "analyze" {
		scheduleAnalyze(args)
	} else {
		fmt.Println("Usage: gojira-cli schedule analyze --project CODE")
	}
}

func scheduleAnalyze(args []string) {
	project := parseFlag(args, "project")
	if project == "" {
		fatal("--project is required")
	}
	result, err := apiPost("/Agent/chat", map[string]interface{}{
		"message": fmt.Sprintf("%s 프로젝트 일정을 분석해줘", project),
		"context": map[string]string{"page": "project", "projectCode": project},
	})
	if err != nil {
		fatal(err.Error())
	}
	rm, _ := result.(map[string]interface{})
	fmt.Println(rm["reply"])
}
