package rules

import (
	"math"
	"testing"
)

func TestAwarenessRejectsInvalidRanges(t *testing.T) {
	a := Awareness{RangeCells: 3, ConcealedCells: 1}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	a.RangeCells = 0
	if a.Validate() == nil {
		t.Fatal("zero range accepted")
	}
	a.RangeCells = math.Inf(1)
	if a.Validate() == nil {
		t.Fatal("infinite range accepted")
	}
	a.RangeCells, a.ConcealedCells = 3, 4
	if a.Validate() == nil {
		t.Fatal("concealment range exceeds sight")
	}
	a.ConcealedCells = -1
	if a.Validate() == nil {
		t.Fatal("negative concealment range accepted")
	}
}
