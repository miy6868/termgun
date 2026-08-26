# Linux 플랫폼과 입력 규칙

현재 개발 범위는 Linux만이다. Windows 파일과 동작은 고려·수정·검증하지 않는다.
사용자가 나중에 Windows 지원을 명시적으로 요청한 경우에만 별도 작업으로 다룬다.

## 파일 대응

| 역할 | 파일 |
|---|---|
| 터미널 제어 | `term_linux.go` |
| 입력 루프 | `input_linux.go` |
| 키 디코딩 | `evdev.go` |
| 테스트 | `evdev_linux_test.go` |

플랫폼 파일은 `enterRaw`, `termState.restore`, `detectKitty`, `terminalSize`, `isTTY`,
`readEvents`, `chooseInput`, `inputEnableSeq`, `inputDisableSeq`를 모두 제공해야 한다.

## 입력

- Linux `auto` 입력은 kitty 프로토콜, `/dev/input`, 호환 모드 순으로 선택한다.
- `/dev/input`에서는 이동키만 읽고 포커스를 잃으면 누른 키를 모두 놓고 입력을 무시한다.
- 호환 모드는 자동반복 지연을 보정하되 마지막으로 누른 반대 방향 키가 이기게 한다.

macOS는 `syscall.TCGETS` 때문에 현재 지원하지 않는다. 검증 환경 없이 지원을 추정해
추가하지 않는다.

입력 또는 터미널 코드를 바꾸면 Linux 테스트와 일반 `go vet ./...`, `go test ./...`만
수행한다. Windows 교차 빌드와 Windows 전용 테스트는 실행하지 않는다.
