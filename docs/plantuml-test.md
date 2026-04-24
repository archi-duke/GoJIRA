# PlantUML 로컬 렌더 테스트

Markdown Preview Enhanced + 로컬 PlantUML Server (localhost:8080) 연동 확인용.

## 1. 패키지 다이어그램

```plantuml
@startuml
skinparam packageStyle rectangle

package "Presentation Layer" {
  [API Gateway]
  [Auth Middleware]
}

package "Domain" {
  [Game Module]
  [Matching Module]
  [Stats Module]
}

package "Infrastructure" {
  [Redis Adapter]
  [DB Adapter]
}

[API Gateway] --> [Auth Middleware]
[Auth Middleware] --> [Game Module]
[Auth Middleware] --> [Matching Module]
[Auth Middleware] --> [Stats Module]
[Game Module] --> [Redis Adapter]
[Matching Module] --> [Redis Adapter]
[Stats Module] --> [DB Adapter]
@enduml
```

## 2. 배포 다이어그램 (한국어 테스트)

```plantuml
@startuml
node "사내망 App Server" as server {
  artifact "Game App (Monolith)" as app
  artifact "Socket.IO Handler" as sio
}

node "Redis" as redis {
  database "Game State" as gs
}

node "Postgres" as db {
  database "User/Stats" as us
}

cloud "사내 SSO" as sso
actor "플레이어" as user

user --> server : HTTPS
server --> sso : 세션 검증
app --> redis : 상태 R/W
app --> db : 영속화
sio --> app
@enduml
```

## 3. 클래스 다이어그램 (UML 엄격)

```plantuml
@startuml
class GameSession {
  -id: UUID
  -players: List<Player>
  -state: GameState
  +start(): void
  +guess(player, number): Result
  +end(): void
}

class Player {
  -id: String
  -name: String
  -wins: int
}

class GameState <<enum>> {
  WAITING
  IN_PROGRESS
  COMPLETED
}

GameSession "1" o-- "2" Player
GameSession --> GameState
@enduml
```

## 4. Use Case 다이어그램 — actor 스타일 비교

### 4.1 기본 (PlantUML 2026 기본 스타일)

```plantuml
@startuml
left to right direction
actor 플레이어 as user
actor 운영자 as admin
rectangle "숫자 야구 게임" {
  usecase "게임 시작" as UC1
  usecase "매칭" as UC2
  usecase "숫자 추리" as UC3
  usecase "전적 조회" as UC4
  usecase "장애 대응" as UC5
}
user --> UC1
user --> UC2
user --> UC3
user --> UC4
admin --> UC5
@enduml
```

### 4.2 전통적 stick figure (skinparam actorStyle stick)

가장 표준적인 UML stick figure — 단순 원 + 막대기.

```plantuml
@startuml
skinparam actorStyle stick
left to right direction
actor 플레이어 as user
actor 운영자 as admin
rectangle "숫자 야구 게임" {
  usecase "게임 시작" as UC1
  usecase "매칭" as UC2
}
user --> UC1
admin --> UC2
@enduml
```

### 4.3 Hollow (사각 박스 + 사람 아이콘)

UML 2.x 에서도 정식 — 박스 안에 작은 stick figure.

```plantuml
@startuml
skinparam actorStyle hollow
left to right direction
actor 플레이어 as user
actor 운영자 as admin
rectangle "숫자 야구 게임" {
  usecase "게임 시작" as UC1
  usecase "매칭" as UC2
}
user --> UC1
admin --> UC2
@enduml
```

### 4.4 Awesome (사람형 아이콘, 가장 예쁜)

```plantuml
@startuml
skinparam actorStyle awesome
left to right direction
actor 플레이어 as user
actor 운영자 as admin
rectangle "숫자 야구 게임" {
  usecase "게임 시작" as UC1
  usecase "매칭" as UC2
}
user --> UC1
admin --> UC2
@enduml
```

### 4.5 strictuml 모드 (UML 엄격)

```plantuml
@startuml
skinparam style strictuml
left to right direction
actor 플레이어 as user
actor 운영자 as admin
rectangle "숫자 야구 게임" {
  usecase "게임 시작" as UC1
  usecase "매칭" as UC2
}
user --> UC1
admin --> UC2
@enduml
```

## 5. 시퀀스 다이어그램 (Mermaid 와 비교용)

```plantuml
@startuml
actor Player1
actor Player2
participant "Socket.IO" as SIO
participant "Game Module" as GM
database Redis

Player1 -> SIO: connect
SIO -> GM: join room
Player2 -> SIO: connect
SIO -> GM: join room
GM -> Redis: create session
GM --> SIO: game start

loop 게임 턴
    Player1 -> SIO: guess(123)
    SIO -> GM: processGuess
    GM -> GM: calc S/B/O
    GM -> Redis: save state
    GM --> SIO: result
    SIO --> Player1: strike/ball
    SIO --> Player2: opponent turn
end
@enduml
```

---

## 사용 방법

1. VS Code 에서 이 파일 열기
2. `Cmd+Shift+V` (또는 우클릭 → Markdown Preview Enhanced: Open Preview)
3. 각 PlantUML 블록이 SVG 로 렌더되는지 확인

## 설정 위치

`.vscode/settings.json` 의 `markdown-preview-enhanced.plantumlServer` 가 `http://localhost:8080/svg/` 로 지정됨.

## 트러블슈팅

| 증상 | 원인 | 조치 |
|---|---|---|
| "Rendering..." 이 계속 보임 | 서버 미기동 | `docker ps` 로 `plantuml-server` 상태 확인 |
| 한글이 박스로 나옴 | 폰트 fallback 문제 | SVG 는 브라우저 폰트 사용 — 실제 preview 에서는 정상 |
| 빈 다이어그램 | PlantUML 문법 오류 | VS Code Output → MPE 로그 확인 |
| HTTP 400 | UTF-8 encoding 실패 | MPE 최신 버전인지 확인 |
