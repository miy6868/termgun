# 시뮬레이션 규칙

## 몸은 벽 안에 들어갈 수 없다

- `moveWithCollision`은 큰 이동을 몸 크기 이하 단계로 나눠 중간 벽 통과를 막는다.
- `addEnemy`와 `addPickup` 안에서 `Level.NearestFree`로 소환 위치를 보정한다. 호출부마다
  따로 보정하지 않는다.
- 매복 문을 닫을 때 `clearOfWalls`로 몸 전체가 움직일 수 있는 자리로 밀어낸다.
- 한 칸 통로에서 한쪽만 막히면 `slip`으로 코너를 보정하되 양쪽이 막히면 보정하지 않는다.
- `FlowStep`은 대각선으로 벽 모서리를 가로지르지 않는다.

## 슬라이스와 포인터

`updateBullets`와 `reapEnemies`는 스크래치 버퍼로 컴팩션하는 동안 새 탄과 적이 추가될
수 있다. 읽는 슬라이스와 쓰는 버퍼가 앨리어싱되어 새 항목을 덮어쓰지 않게 한다.

`addEnemy`가 슬라이스를 재할당할 수 있으므로 호출 전의 `*Enemy` 포인터를 호출 후까지
보관하지 않는다. 인덱스나 값을 다시 조회한다.

## 던전과 지형

- 새 타일을 추가하면 `solid()`와 `plainFloor()`를 함께 확인한다.
- `Break`나 `SetTiles`처럼 지형을 바꾸는 경로는 `invalidateTerrain()`으로 flow field와
  FOV 캐시를 같은 프레임에 무효화한다.
- `Openings()`는 `plainFloor`만 문으로 바꿔 계단을 봉쇄하지 않는다.
- 산성은 시작 지점과 도착 즉시 피해를 주는 위치에 배치하지 않는다.

## 체력 경제

- 적 하나를 죽여 얻는 기대 회복량은 그 적에게 한 대 맞는 피해보다 작아야 한다.
- 흡혈은 요청 피해가 아니라 실제로 깎인 체력(`landed`)만 기준으로 지급한다.
- 체력을 전부 채우는 수단을 추가하지 않는다.
- 산성 피해는 일반 피해의 무적 시간을 부여하지 않는다.
- 모든 회복은 최대 체력을 넘지 않는다.

관련 테스트: `walls_test.go`, `doorway_test.go`, `props_test.go`, `rooms_test.go`,
`balance_test.go`, `elite_test.go`, `telegraph_test.go`, `fov_test.go`.
