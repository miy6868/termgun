# termgun 작업 지침

`termgun`은 Go 표준 라이브러리만 사용하는 터미널 실시간 트윈스틱 로그라이크
슈터다. 모든 코드는 `package main` 단일 패키지에 있다.

게임 사용법과 기본 규칙은 [README.md](README.md), 코드 변경의 세부 불변식은
[`rules/`](rules/)에 있다. 코드 작업에는 저장소 스킬
`$termgun-development`를 사용하고, 작업 영역에 해당하는 규칙만 읽는다.

## 필수 규칙

- 외부 의존성을 추가하지 않는다. Go 표준 라이브러리만 사용한다.
- `math.go`의 `aspect = 2.0`과 `Vec.visual()` / `.unvisual()` 좌표계를 보존한다.
- 몸이 벽 안에 들어가거나 대각선으로 벽 모서리를 통과하게 만들지 않는다.
- 지형 변경은 flow field와 FOV 캐시를 함께 무효화한다.
- 순회 중 append되는 슬라이스의 앨리어싱과 `addEnemy` 전후의 `*Enemy` 포인터를
  주의한다.
- UI 문자열은 한국어, 코드 주석은 영어로 쓴다. 주석은 무엇보다 이유를 설명한다.
- 플레이 영역 글리프는 ASCII만 사용하고 기존 글리프와 충돌하지 않게 한다.
- 새 기능은 HUD, 도움말 또는 글리프로 화면에 드러나야 한다.
- 튜닝 값은 파일 상단의 이름 있는 상수로 둔다.
- 사용자가 명시적으로 windows 버전을 요청하지 않은이상 linux 버전을 작업한다.

## 환경과 검증

Go가 기본 PATH에 없으므로 먼저 실행한다.

```sh
export PATH="$HOME/.local/go/bin:$PATH"
```

전체 검증:

```sh
gofmt -l . && go vet ./... && go test -count=1 ./...
```

필요할 때만 사용한다.

```sh
go test -run TestName -v .
go test -run XXX -bench . -benchmem .
go build -o termgun . && ./termgun -seed 42 -zoom 2
```

## 규칙 라우팅

| 작업 | 먼저 읽을 문서 | 핵심 테스트 |
|---|---|---|
| 구조·파일 위치·관례 | [rules/repository.md](rules/repository.md) | 관련 패키지 테스트 |
| 그리기·카메라·배율·문자 폭·성능 | [rules/rendering.md](rules/rendering.md) | `draw_test.go` `camera_test.go` `zoom_test.go` `render_test.go` |
| 이동·충돌·소환·던전·체력·적 | [rules/simulation.md](rules/simulation.md) | `walls_test.go` `doorway_test.go` `balance_test.go` `elite_test.go` |
| 입력·터미널·Linux | [rules/platform.md](rules/platform.md) | `move_test.go` `evdev_linux_test.go` |
| 테스트 작성·재현·소크 | [rules/testing.md](rules/testing.md) | 해당 문서의 최소 관련 테스트 |
| 게임 규칙·밸런스 수치 | [rules/gameplay.md](rules/gameplay.md) | `balance_test.go` 및 관련 기능 테스트 |

사용자 보고에는 확인한 것과 확인하지 못한 것을 구분한다. Windows 지원 상태는 이번
작업 범위에 포함하지 않는다.
