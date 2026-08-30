package main

const (
	floorsPerAct  = 5
	campaignActs  = 3
	campaignDepth = floorsPerAct * campaignActs
)

var actNames = [campaignActs]string{"정비 구역", "배양 구역", "반응로 구역"}

func actForDepth(depth int) int {
	return clamp((depth-1)/floorsPerAct, 0, campaignActs-1)
}

func actName(depth int) string { return actNames[actForDepth(depth)] }

// actPalette changes only presentation. Terrain rules stay shared, while each
// five-floor chapter gets an immediate terminal-readable identity.
func actPalette(depth int) (wall, floor, floorMid int16) {
	switch actForDepth(depth) {
	case 1:
		return 97, 29, 23 // wet violet walls and biolab green floors
	case 2:
		return 45, 31, 24 // cold cyan machinery over a dark reactor deck
	default:
		return colWallLit, colFloorLit, 238
	}
}
