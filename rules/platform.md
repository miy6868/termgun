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
- 호환 모드는 자동반복 지연을 보정하되 마지막으로 누른 반대 방향 키가 이기게 한다.
- 이벤트 채널은 여러 생산자가 공유하므로 생산자가 닫지 않는다. 주 입력 소스 종료는
  `EvStop`으로 알린다.
- Linux `SIGTERM`과 `SIGHUP`은 `EvStop`으로 바꾸어 터미널 복구 후 종료한다.
- evdev 정수는 커널의 native endian으로 디코딩한다.

macOS는 `syscall.TCGETS` 때문에 현재 지원하지 않는다. 검증 환경 없이 지원을 추정해
추가하지 않는다.

입력 또는 터미널 코드를 바꾸면 Linux 테스트와 일반 `go vet ./...`, `go test ./...`를
수행한다. Windows 작업에서는 `GOOS=windows` 교차 빌드와 플랫폼 독립적인 Windows
콘솔 디코딩 테스트도 수행하되, 실제 콘솔 동작을 확인하지 못했다면 보고에 명시한다.
