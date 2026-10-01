package game

import "math"

// Hex helpers for the pointy-top, odd-row-offset grid ("odd-r").

func toCube(col, row int) (x, y, z int) {
	x = col - (row-(row&1))/2
	z = row
	return x, -x - z, z
}

// HexDistance is the number of hex steps between two hexes.
func HexDistance(c1, r1, c2, r2 int) int {
	x1, y1, z1 := toCube(c1, r1)
	x2, y2, z2 := toCube(c2, r2)
	return max(abs(x1-x2), abs(y1-y2), abs(z1-z2))
}

var oddRNeighbors = [2][6][2]int{
	{{+1, 0}, {0, -1}, {-1, -1}, {-1, 0}, {-1, +1}, {0, +1}}, // even rows
	{{+1, 0}, {+1, -1}, {0, -1}, {-1, 0}, {0, +1}, {+1, +1}}, // odd rows
}

// Compass gives the 8-point direction from one hex to another, for reports.
func Compass(fromCol, fromRow, toCol, toRow int) string {
	x1, y1 := HexCenter(fromCol, fromRow)
	x2, y2 := HexCenter(toCol, toRow)
	if x1 == x2 && y1 == y2 {
		return "center"
	}
	a := math.Atan2(y1-y2, x2-x1) * 180 / math.Pi // screen y grows downward
	names := []string{"east", "north-east", "north", "north-west", "west", "south-west", "south", "south-east"}
	i := int(math.Round(a/45)+8) % 8
	return names[i]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
