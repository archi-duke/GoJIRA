이 디렉터리는 gojira-launcher exe 에 임베드되는 자산(skills 등)의 루트입니다.

- 빌드 시 deploy/scripts/build-launcher.sh 가 레포 루트 .claude 를
  이 디렉터리(assets/.claude)로 복사한 뒤 `go build` 합니다.
- assets/.claude 는 .gitignore 로 제외됩니다(소스는 레포 루트 .claude 가 단일 진실).
- 이 README.txt 는 embed 디렉터리가 항상 비어있지 않도록 보장하는 placeholder 입니다
  (go:embed 는 매칭 파일이 없으면 컴파일 에러).
