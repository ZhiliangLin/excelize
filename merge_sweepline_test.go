// Copyright 2016 - 2026 The excelize Authors. All rights reserved. Use of
// this source code is governed by a BSD-style license that can be found in
// the LICENSE file.

package excelize

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// referenceMergeRectList is a verbatim copy of the former quadratic
// implementation of mergeOverlapCells. Merging until no rectangle overlaps
// another has several valid end states: which one an algorithm reaches
// depends on the order in which it examines rectangles, so the sweep line
// implementation is not expected to match this reference verbatim on every
// input. The reference is kept to document and observe the difference.
func referenceMergeRectList(rectList [][]int) [][]int {
	merged := true
	for merged {
		merged = false
		var mergedRectList [][]int
		used := make([]bool, len(rectList))
		for i := 0; i < len(rectList); i++ {
			if used[i] {
				continue
			}
			r1 := rectList[i]
			for j := i + 1; j < len(rectList); j++ {
				if used[j] {
					continue
				}
				if r2 := rectList[j]; isOverlap(r1, r2) {
					r1 = []int{
						min(r1[0], r2[0]),
						min(r1[1], r2[1]),
						max(r1[2], r2[2]),
						max(r1[3], r2[3]),
					}
					merged, used[j] = true, true
				}
			}
			mergedRectList = append(mergedRectList, r1)
		}
		rectList = mergedRectList
	}
	return rectList
}

func TestMergeRectList(t *testing.T) {
	for _, test := range []struct {
		rects    [][]int
		expected [][]int
	}{
		// no rectangles
		{rects: nil, expected: nil},
		{rects: [][]int{{0, 0, 1, 1}}, expected: [][]int{{0, 0, 1, 1}}},
		// disjoint rectangles keep their order
		{
			rects:    [][]int{{5, 5, 6, 6}, {0, 0, 1, 1}, {10, 10, 11, 11}},
			expected: [][]int{{5, 5, 6, 6}, {0, 0, 1, 1}, {10, 10, 11, 11}},
		},
		// vertically adjacent rectangles sharing no row are not merged
		{
			rects:    [][]int{{0, 0, 2, 0}, {0, 1, 2, 1}},
			expected: [][]int{{0, 0, 2, 0}, {0, 1, 2, 1}},
		},
		// vertically overlapping rectangles sharing a row are merged
		{
			rects:    [][]int{{0, 0, 2, 1}, {1, 1, 3, 2}},
			expected: [][]int{{0, 0, 3, 2}},
		},
		// horizontally adjacent rectangles sharing no column are not merged
		{
			rects:    [][]int{{0, 0, 2, 2}, {3, 0, 5, 2}},
			expected: [][]int{{0, 0, 2, 2}, {3, 0, 5, 2}},
		},
		// horizontally overlapping rectangles sharing a column are merged
		{
			rects:    [][]int{{0, 0, 2, 2}, {2, 0, 4, 2}},
			expected: [][]int{{0, 0, 4, 2}},
		},
		// identical rectangles are merged
		{
			rects:    [][]int{{2, 2, 5, 5}, {2, 2, 5, 5}},
			expected: [][]int{{2, 2, 5, 5}},
		},
		// a rectangle fully containing another one is merged
		{
			rects:    [][]int{{0, 0, 9, 9}, {2, 3, 4, 5}},
			expected: [][]int{{0, 0, 9, 9}},
		},
		// a vertical chain of rectangles merges into one
		{
			rects:    [][]int{{0, 0, 2, 1}, {0, 1, 2, 2}, {0, 2, 2, 3}},
			expected: [][]int{{0, 0, 2, 3}},
		},
		// merging grows a rectangle so that it overlaps one that shares no
		// row or column with any of its members directly
		{
			rects:    [][]int{{0, 0, 2, 4}, {1, 4, 10, 5}, {5, 0, 6, 1}},
			expected: [][]int{{0, 0, 10, 5}},
		},
	} {
		assert.Equal(t, test.expected, mergeRectList(test.rects), "sweep-line result")
		assert.Equal(t, test.expected, referenceMergeRectList(test.rects), "reference result")
	}
}

// mergeRectListResultValid checks the two invariants that any correct merge
// result must satisfy: no two returned rectangles overlap each other, and
// every input rectangle is contained by exactly one returned rectangle whose
// bounding box equals the bounding box of exactly the inputs it contains.
func mergeRectListResultValid(t *testing.T, rects, result [][]int) {
	for a := 0; a < len(result); a++ {
		for b := a + 1; b < len(result); b++ {
			assert.False(t, isOverlap(result[a], result[b]),
				"result rectangles %v and %v overlap", result[a], result[b])
		}
	}
	owner := make([]int, len(rects))
	for i := range owner {
		owner[i] = -1
	}
	for k, r := range result {
		box := []int{r[0], r[1], r[2], r[3]}
		for i, rect := range rects {
			if rect[0] >= r[0] && rect[1] >= r[1] && rect[2] <= r[2] && rect[3] <= r[3] {
				assert.Equal(t, -1, owner[i], "input %v contained by more than one result", rect)
				owner[i] = k
				box = []int{
					min(box[0], rect[0]),
					min(box[1], rect[1]),
					max(box[2], rect[2]),
					max(box[3], rect[3]),
				}
			}
		}
		assert.Equal(t, box, r, "result %v differs from the bounding box of its inputs", r)
	}
	for i := range owner {
		assert.NotEqual(t, -1, owner[i], "input %v not contained by any result", rects[i])
	}
}

// TestMergeRectListEquivalenceRandom verifies the sweep-line implementation
// on randomized rectangle sets, both dense ones that force many overlaps and
// merges, and sparse ones. Every result is checked for validity, and results
// are compared verbatim with the former quadratic algorithm whenever that
// algorithm terminates at a partition the sweep line agrees with. The former
// algorithm is order dependent and may stop early while rectangles still
// grow into each other, so verbatim agreement is not expected in every case;
// such cases are logged for inspection.
func TestMergeRectListEquivalenceRandom(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	gen := func(maxX, maxY, n int) [][]int {
		rects := make([][]int, n)
		for i := range rects {
			x1, y1 := rnd.Intn(maxX), rnd.Intn(maxY)
			rects[i] = []int{
				x1, y1,
				min(x1+rnd.Intn(4)+1, maxX-1),
				min(y1+rnd.Intn(4)+1, maxY-1),
			}
		}
		return rects
	}
	stable := func(result [][]int) bool {
		for a := 0; a < len(result); a++ {
			for b := a + 1; b < len(result); b++ {
				if isOverlap(result[a], result[b]) {
					return false
				}
			}
		}
		return true
	}
	diverged := 0
	for _, combo := range []struct{ maxX, maxY int }{
		{6, 6}, {12, 8}, {30, 20},
	} {
		for _, n := range []int{0, 1, 2, 3, 5, 10, 30} {
			for trial := 0; trial < 200; trial++ {
				rects := gen(combo.maxX, combo.maxY, n)
				result := mergeRectList(rects)
				mergeRectListResultValid(t, rects, result)
				if ref := referenceMergeRectList(rects); !equalRectList(ref, result) {
					diverged++
					if stable(ref) {
						t.Logf("different but stable results for rects %v: former %v, sweep line %v",
							rects, ref, result)
					} else {
						t.Logf("former algorithm kept mergeable rectangles: %v -> %v", rects, ref)
					}
				}
			}
		}
	}
	// sparse medium-sized sets with occasional overlaps
	for trial := 0; trial < 50; trial++ {
		rects := gen(200, 150, 150)
		result := mergeRectList(rects)
		mergeRectListResultValid(t, rects, result)
		if ref := referenceMergeRectList(rects); !equalRectList(ref, result) {
			diverged++
			if stable(ref) {
				t.Logf("different but stable results for rects %v: former %v, sweep line %v",
					rects, ref, result)
			} else {
				t.Logf("former algorithm kept mergeable rectangles: %v -> %v", rects, ref)
			}
		}
	}
	t.Logf("results differ from the former algorithm in %d of %d cases", diverged, 4200+50)
}

// equalRectList reports whether the two rectangle lists are equal.
func equalRectList(a, b [][]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for k := range a[i] {
			if a[i][k] != b[i][k] {
				return false
			}
		}
	}
	return true
}

// TestMergeRectListLargeScale checks that merging a large number of
// non-overlapping rectangles, the dominant case when generating big reports,
// completes quickly and keeps every rectangle untouched.
func TestMergeRectListLargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-scale test in short mode")
	}
	const n = 200000
	rects := make([][]int, n)
	for i := range rects {
		rects[i] = []int{0, i, 1, i} // one merged range per row
	}
	start := time.Now()
	result := mergeRectList(rects)
	assert.Len(t, result, n)
	for i, r := range result {
		assert.Equal(t, []int{0, i, 1, i}, r)
	}
	t.Logf("merged %d disjoint rectangles in %v", n, time.Since(start))
}

// TestMergeRectListLargeScaleChain checks that a tall vertical chain of
// overlapping rectangles, which requires several sweep passes, merges into a
// single rectangle in reasonable time.
func TestMergeRectListLargeScaleChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-scale test in short mode")
	}
	const n = 20000
	rects := make([][]int, n)
	for i := range rects {
		rects[i] = []int{0, i, 2, i + 1}
	}
	start := time.Now()
	result := mergeRectList(rects)
	assert.Equal(t, [][]int{{0, 0, 2, n}}, result)
	t.Logf("merged %d chained rectangles in %v", n, time.Since(start))
}

// TestMergeCellSweepLine verifies merged cell overlap handling end to end
// through the worksheet writer.
func TestMergeCellSweepLine(t *testing.T) {
	f := NewFile()
	var expected [][]string
	for i := 0; i < 1000; i++ {
		cell, _ := CoordinatesToCellName(i%20+1, i/20+1)
		assert.NoError(t, f.MergeCell("Sheet1", cell, cell))
		expected = append(expected, []string{cell + ":" + cell, ""})
	}
	assert.NoError(t, f.SaveAs(filepath.Join(t.TempDir(), "sweepline.xlsx")))
	mergeCells, err := f.GetMergeCells("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, mergeCells, 1000)
	sort.Slice(mergeCells, func(i, j int) bool { return mergeCells[i][0] < mergeCells[j][0] })
	sort.Slice(expected, func(i, j int) bool { return expected[i][0] < expected[j][0] })
	// Convert []MergeCell to [][]string: testify requires identical dynamic
	// types, and MergeCell is a named []string type.
	actual := make([][]string, len(mergeCells))
	for i, mc := range mergeCells {
		actual[i] = []string(mc)
	}
	assert.Equal(t, expected, actual)
}

// TestMergeCellsParserIndex checks the row-keyed merge range index used by
// mergeCellsParser: cells covered by a merge resolve to the top-left cell of
// the range, uncovered cells stay unchanged, and the index is correctly
// invalidated when the merge cells change afterwards.
func TestMergeCellsParserIndex(t *testing.T) {
	f := NewFile()
	assert.NoError(t, f.MergeCell("Sheet1", "A1", "C1"))  // single-row range
	assert.NoError(t, f.MergeCell("Sheet1", "B2", "B10")) // multi-row range
	assert.NoError(t, f.MergeCell("Sheet1", "E5", "E5"))  // single cell
	for _, c := range []string{"a1", "B1", "c1"} {
		cell, err := wsOf(f, "Sheet1").mergeCellsParser(c)
		assert.NoError(t, err)
		assert.Equal(t, "A1", cell, "cell %s", c)
	}
	cell, err := wsOf(f, "Sheet1").mergeCellsParser("B7")
	assert.NoError(t, err)
	assert.Equal(t, "B2", cell)
	cell, err = wsOf(f, "Sheet1").mergeCellsParser("e5")
	assert.NoError(t, err)
	assert.Equal(t, "E5", cell)
	cell, err = wsOf(f, "Sheet1").mergeCellsParser("D1")
	assert.NoError(t, err)
	assert.Equal(t, "D1", cell)

	// A merge added after the index has been built must be found.
	cell, err = wsOf(f, "Sheet1").mergeCellsParser("H1")
	assert.NoError(t, err)
	assert.Equal(t, "H1", cell)
	assert.NoError(t, f.MergeCell("Sheet1", "G1", "I1"))
	cell, err = wsOf(f, "Sheet1").mergeCellsParser("H1")
	assert.NoError(t, err)
	assert.Equal(t, "G1", cell)

	// Unmerging must invalidate the index as well.
	assert.NoError(t, f.UnmergeCell("Sheet1", "B2", "B10"))
	cell, err = wsOf(f, "Sheet1").mergeCellsParser("B7")
	assert.NoError(t, err)
	assert.Equal(t, "B7", cell)

	// Row insertion moves merge ranges and must invalidate the index:
	// A1:C1 shifts to A3:C3, so B3 resolves to A3 and row 1 is uncovered.
	assert.NoError(t, f.InsertRows("Sheet1", 1, 2))
	cell, err = wsOf(f, "Sheet1").mergeCellsParser("B3")
	assert.NoError(t, err)
	assert.Equal(t, "A3", cell)
	cell, err = wsOf(f, "Sheet1").mergeCellsParser("A1")
	assert.NoError(t, err)
	assert.Equal(t, "A1", cell)
}

// wsOf loads the cached worksheet structure of the given sheet.
func wsOf(f *File, sheet string) *xlsxWorksheet {
	name, _ := f.getSheetXMLPath(sheet)
	ws, _ := f.Sheet.Load(name)
	return ws.(*xlsxWorksheet)
}

// TestMergeCellInterleavedWrites verifies that interleaving cell writes with
// merge calls on a large worksheet, the pattern that used to degrade to
// O(cells x merges) because every write scanned all merge ranges, finishes
// quickly and keeps the expected values.
func TestMergeCellInterleavedWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-scale test in short mode")
	}
	const rows = 50000
	f := NewFile()
	start := time.Now()
	for r := 1; r <= rows; r++ {
		a, _ := CoordinatesToCellName(1, r)
		h, _ := CoordinatesToCellName(8, r)
		for c := 1; c <= 8; c++ {
			cell, _ := CoordinatesToCellName(c, r)
			assert.NoError(t, f.SetCellValue("Sheet1", cell, fmt.Sprintf("r%d-c%d", r, c)))
		}
		assert.NoError(t, f.MergeCell("Sheet1", a, h))
	}
	t.Logf("built %d rows with interleaved writes and merges in %v", rows, time.Since(start))
	start = time.Now()
	assert.NoError(t, f.SaveAs(filepath.Join(t.TempDir(), "interleaved.xlsx")))
	t.Logf("saved in %v", time.Since(start))
	// Merged cells resolve to the value of their top-left cell.
	v, err := f.GetCellValue("Sheet1", "C1")
	assert.NoError(t, err)
	assert.Equal(t, "r1-c1", v)
	mergeCells, err := f.GetMergeCells("Sheet1")
	assert.NoError(t, err)
	assert.Len(t, mergeCells, rows)
}
