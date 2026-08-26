---
name: termgun-development
description: Safely modify, debug, review, or test the termgun Go game. Use for any code change involving rendering, camera, zoom, terminal width, movement, collision, spawning, enemies, dungeon generation, balance, input, terminal handling, performance, or tests in this repository.
---

# termgun 개발

1. 요청과 관련 파일의 실제 흐름을 끝까지 읽고, 수정할 함수의 모든 호출부를 찾는다.
2. 아래 표에서 직접 관련된 규칙 문서만 읽는다. 여러 영역을 건드릴 때만 여러 문서를
   읽는다.
3. 공통 경로의 근본 원인을 가장 작은 변경으로 수정한다. 새 의존성과 추상화는 추가하지
   않는다.
4. 기존 관련 테스트를 먼저 실행하고, 보호되지 않은 비자명한 로직에만 작은 회귀 테스트
   하나를 남긴다.
5. 관련 테스트가 통과하면 `AGENTS.md`의 전체 검증을 실행한다.

| 작업 | 규칙 |
|---|---|
| 저장소 구조·관례 | [repository.md](../../../rules/repository.md) |
| 렌더링·카메라·배율·문자 폭·성능 | [rendering.md](../../../rules/rendering.md) |
| 이동·충돌·소환·던전·체력·적 | [simulation.md](../../../rules/simulation.md) |
| 입력·터미널·운영체제 | [platform.md](../../../rules/platform.md) |
| 재현·회귀 테스트·소크 | [testing.md](../../../rules/testing.md) |
| 게임 규칙·밸런스 수치 | [gameplay.md](../../../rules/gameplay.md) |

보고할 때 실행한 검증과 실행하지 못한 검증을 구분한다.
