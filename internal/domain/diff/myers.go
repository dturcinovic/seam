package diff

import (
	"context"
	"slices"
)

// match pairs an old element, a line or a token, with an equal new one.
type match struct {
	old, new int
}

// myers finds a longest common subsequence of two sequences of ids with
// Myers' algorithm in linear space: it bisects at the middle snake of an
// optimal path and recurses into both halves. The bisection follows
// diff-match-patch's diff_bisect.
type myers struct {
	ctx     context.Context
	a, b    []int
	v1, v2  []int
	matches []match
	stopped bool
	// walk is reused, as bisections run one at a time: a fresh one each time
	// costs an allocation per bisection.
	walk walk
}

// commonSubsequence returns the matched pairs in order. Once ctx ends, no
// further search starts; the ranges left are reported without matches, as
// replaced blocks, and complete is false.
func commonSubsequence(ctx context.Context, a, b []int) (matches []match, complete bool) {
	// Without an element in common the search would spend quadratic time, or
	// the whole budget, to find nothing; a rewritten file or a change of line
	// endings is exactly that.
	if !slices.ContainsFunc(b, func(id int) bool { return id != noMatch }) {
		return nil, true
	}
	m := &myers{ctx: ctx, a: a, b: b, matches: make([]match, 0, min(len(a), len(b)))}
	m.compare(0, len(a), 0, len(b))
	return m.matches, !m.stopped
}

// compare matches a[a0:a1] against b[b0:b1].
func (m *myers) compare(a0, a1, b0, b1 int) {
	for a0 < a1 && b0 < b1 && m.a[a0] == m.b[b0] {
		m.matches = append(m.matches, match{a0, b0})
		a0, b0 = a0+1, b0+1
	}
	suffix := 0
	for a0 < a1-suffix && b0 < b1-suffix && m.a[a1-1-suffix] == m.b[b1-1-suffix] {
		suffix++
	}
	a1, b1 = a1-suffix, b1-suffix
	if a0 < a1 && b0 < b1 {
		if x, y, ok := m.bisect(a0, a1, b0, b1); ok {
			m.compare(a0, a0+x, b0, b0+y)
			m.compare(a0+x, a1, b0+y, b1)
		}
	}
	for i := range suffix {
		m.matches = append(m.matches, match{a1 + i, b1 + i})
	}
}

// bisect returns a point, relative to a0 and b0, on an optimal path through
// the middle of the edit graph of a[a0:a1] against b[b0:b1], walking from
// both ends until the paths overlap. ok is false if ctx ended first or the
// range has no element in common.
func (m *myers) bisect(a0, a1, b0, b1 int) (x, y int, ok bool) {
	w := m.newWalk(m.a[a0:a1], m.b[b0:b1])
	// With an odd delta the forward walk meets the backward one, otherwise
	// the backward walk meets the forward one.
	odd := w.delta%2 != 0
	for d := range w.maxD {
		if m.ctx.Err() != nil {
			m.stopped = true
			return 0, 0, false
		}
		if x, y, met := w.extend(d, &w.fwd, &w.bwd, odd); met {
			return m.split(w, x, y)
		}
		if x, y, met := w.extend(d, &w.bwd, &w.fwd, !odd); met {
			return m.split(w, x, y)
		}
	}
	// The paths never met: the range has no element in common and is one
	// replaced block.
	return 0, 0, false
}

// split accepts a point that divides the range. A corner would recurse until
// the stack overflows, leaving the terminal raw; trimming rules it out, and
// should that break, the range stays one block and the diff says simplified.
func (m *myers) split(w *walk, x, y int) (int, int, bool) {
	if (x == 0 && y == 0) || (x == w.n && y == w.m) {
		m.stopped = true
		return 0, 0, false
	}
	return x, y, true
}

// walk is the state of one bisection over a against b.
type walk struct {
	a, b        []int
	n, m        int
	delta, maxD int
	fwd, bwd    frontier
}

// frontier is one direction of a walk. v[maxD+k] is the furthest x reached
// on diagonal k, counted from the start for the forward walk and from the
// end for the backward one; -1 means not reached. Diagonals below kStart and
// above kEnd ran off the edit graph and are skipped.
type frontier struct {
	v            []int
	kStart, kEnd int
	backward     bool
}

// vLen is the length of a frontier's v. Diagonals -maxD..maxD-1 need 2*maxD
// slots, but the start value on diagonal 1 needs maxD+2, which is more when
// maxD is 1.
func vLen(maxD int) int {
	return max(2*maxD, maxD+2)
}

// newWalk starts a bisection. The arrays are allocated on the first one,
// after trimming: later ranges lie within it and need less.
func (m *myers) newWalk(a, b []int) *walk {
	maxD := (len(a) + len(b) + 1) / 2
	n := vLen(maxD)
	if len(m.v1) < n {
		m.v1, m.v2 = make([]int, n), make([]int, n)
	}
	m.walk = walk{
		a: a, b: b, n: len(a), m: len(b),
		delta: len(a) - len(b), maxD: maxD,
		fwd: frontier{v: m.v1[:n]}, bwd: frontier{v: m.v2[:n], backward: true},
	}
	w := &m.walk
	for i := range n {
		w.fwd.v[i], w.bwd.v[i] = -1, -1
	}
	w.fwd.v[maxD+1], w.bwd.v[maxD+1] = 0, 0
	return w
}

// extend moves frontier f by one edit d. If check is set, it looks for the
// overlap with other and returns the point where the forward path meets it.
func (w *walk) extend(d int, f, other *frontier, check bool) (x, y int, met bool) {
	for k := -d + f.kStart; k <= d-f.kEnd; k += 2 {
		x := furthest(f.v, w.maxD+k, k, d)
		y := x - k
		for x < w.n && y < w.m && w.equal(x, y, f.backward) {
			x, y = x+1, y+1
		}
		f.v[w.maxD+k] = x
		switch {
		case x > w.n:
			f.kEnd += 2
		case y > w.m:
			f.kStart += 2
		case check:
			k2 := w.delta - k
			if x2, ok := w.reached(other.v, k2); ok && x+x2 >= w.n {
				if f.backward {
					return x2, x2 - k2, true
				}
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// equal compares the x-th old with the y-th new line, from the end if
// backward.
func (w *walk) equal(x, y int, backward bool) bool {
	if backward {
		return w.a[w.n-1-x] == w.b[w.m-1-y]
	}
	return w.a[x] == w.b[y]
}

// furthest is where diagonal k (at v[i]) starts at edit d: one down from k+1
// or one right from k-1, whichever got further.
func furthest(v []int, i, k, d int) int {
	if k == -d || (k != d && v[i-1] < v[i+1]) {
		return v[i+1]
	}
	return v[i-1] + 1
}

// reached is the other walk's furthest x on its diagonal k, if it has got
// there yet; k may lie beyond the diagonals it can reach at all.
func (w *walk) reached(v []int, k int) (int, bool) {
	i := w.maxD + k
	if i < 0 || i >= len(v) || v[i] == -1 {
		return 0, false
	}
	return v[i], true
}
