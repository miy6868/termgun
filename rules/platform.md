# Linux·Windows 플랫폼과 입력 규칙

기본 개발 범위는 Linux다. 사용자가 Windows 지원을 명시적으로 요청한 작업에서는
Windows 파일과 교차 빌드를 함께 검증한다.

## 파일 대응

| 역할 | 파일 |
|---|---|
| 공통 플랫폼 세션 | `platform.go` |
| 터미널 제어 | `term_linux.go` |
| 입력 루프 | `input_linux.go` |
| 키 디코딩 | `evdev.go` |
| 테스트 | `evdev_linux_test.go` |
| Windows 터미널 제어 | `term_windows.go` |
| Windows 입력 루프 | `input_windows.go` |
| Windows 콘솔 디코딩 | `wincon.go` |
| Windows 입력 테스트 | `wincon_test.go` |

플랫폼 파일은 `enterRaw`, `termState.restore`, `detectKitty`, `terminalSize`, `isTTY`,
`readEvents`, `chooseInput`, `inputEnableSeq`, `inputDisableSeq`를 모두 제공해야 한다.

## 입력

- Linux `auto` 입력은 kitty 프로토콜, `/dev/input`, 호환 모드 순으로 선택한다.
- `/dev/input`에서는 이동키만 읽고 포커스를 잃으면 누른 키를 모두 놓고 입력을 무시한다.
- 모든 비플레이 상태에서 `/dev/input` 방향 press/repeat은 상태나 이동을 바꾸지 않지만
  release는 기존 이동 상태를 반드시 해제한다. 터미널 이동키도 비플레이 상태에서 새로
  눌림 상태를 만들지 않는다.
- kitty와 Windows가 구분해 주는 비이동 키 repeat은 상태 토글이나 행동을 다시 실행하지
  않는다. 이동키와 메뉴 방향키 repeat만 유지한다.
- Linux 터미널의 단독 ESC는 짧은 입력 유휴 시간 뒤 완성된 키로 전달한다. 다음 키를
  기다리게 두면 `/dev/input` 모드의 일시정지 조작이 서로 붙는다.
- 터미널 기능 감지 중 보존한 입력은 새 입력을 기다리지 않고 이벤트 리더 시작 시 먼저
  파싱한다.
- 호환 모드는 자동반복 지연을 보정하되 마지막으로 누른 반대 방향 키가 이기게 한다.
  키 해제를 알 수 없는 레거시 비상 모드이므로 정상 플레이나 새 조작 설계의 기준으로
  삼지 않는다. 기준 입력은 kitty, `/dev/input`, Windows 콘솔이다.
- 이벤트 채널은 여러 생산자가 공유하므로 생산자가 닫지 않는다. 주 입력 소스 종료는
  `EvStop`으로 알린다.
- 마우스 휠 방향은 Linux SGR과 Windows 콘솔에서 같은 버튼 이벤트로 보존해 사용 가능한
  무기를 순환한다. 우클릭이나 중클릭을 놓는 동작이 좌클릭 연사를 취소하면 안 된다.
- Linux `SIGTERM`과 `SIGHUP`은 `EvStop`으로 바꾸어 터미널 복구 후 종료한다.
- evdev 정수는 커널의 native endian으로 디코딩한다.

macOS는 `syscall.TCGETS` 때문에 현재 지원하지 않는다. 검증 환경 없이 지원을 추정해
추가하지 않는다.

입력 또는 터미널 코드를 바꾸면 Linux 테스트와 일반 `go vet ./...`, `go test ./...`를
수행한다. Windows 작업에서는 `GOOS=windows` 교차 빌드와 플랫폼 독립적인 Windows
콘솔 디코딩 테스트도 수행하되, 실제 콘솔 동작을 확인하지 못했다면 보고에 명시한다.
