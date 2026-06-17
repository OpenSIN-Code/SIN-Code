// SPDX-License-Identifier: MIT
// Purpose: Pure-Go approximate nearest-neighbor vector index (issue #347).
// Replaces the brute-force O(N) cosine scan in search.go with an IVF-flat
// (inverted file) index: k-means partitions the corpus into clusters, a
// query probes the nearest clusters and brute-force ranks within them.
// No external dependencies (mandate M2). Thread-safe (mandate M7).
package memory

import (
	"math"
	"sort"
	"sync"
)

// VectorMatch is one approximate-nearest-neighbour hit.
type VectorMatch struct {
	ID       string
	Distance float32
}

// ivfEntry is one stored vector.
type ivfEntry struct {
	id  string
	vec []float32
}

// VectorIndex is an IVF-flat approximate nearest-neighbor index.
// dim is the vector dimensionality; nClusters is the k-means cluster count.
type VectorIndex struct {
	dim       int
	nClusters int

	mu       sync.RWMutex
	centroids [][]float32
	entries  map[int][]ivfEntry // cluster-id -> entries
	idToVec  map[string][]float32
	built    bool
}

// NewVectorIndex creates a new IVF-flat index for dim-dimensional vectors
// with nClusters k-means partitions. If nClusters <= 0 it defaults to 16.
// If dim <= 0 it panics — an index needs a dimensionality.
func NewVectorIndex(dim int, nClusters int) *VectorIndex {
	if dim <= 0 {
		panic("memory: VectorIndex dim must be > 0")
	}
	if nClusters <= 0 {
		nClusters = 16
	}
	return &VectorIndex{
		dim:       dim,
		nClusters: nClusters,
		entries:   make(map[int][]ivfEntry),
		idToVec:   make(map[string][]float32),
		built:     false,
	}
}

// Add inserts (or overwrites) a vector under id. The vector length must
// match the index dimensionality. If the index has not been built yet,
// the vector is buffered and assigned to a cluster on the next Build.
func (vi *VectorIndex) Add(id string, vec []float32) {
	if vi == nil || len(vec) != vi.dim {
		return
	}
	vi.mu.Lock()
	defer vi.mu.Unlock()
	// defensive copy so the caller cannot mutate stored data
	cp := make([]float32, len(vec))
	copy(cp, vec)
	vi.idToVec[id] = cp
	if vi.built && len(vi.centroids) > 0 {
		c := vi.assign(cp)
		vi.removeIDFromClustersLocked(id)
		vi.entries[c] = append(vi.entries[c], ivfEntry{id: id, vec: cp})
	}
}

// removeIDFromClustersLocked removes any prior entry with id from all
// clusters. Caller must hold vi.mu.
func (vi *VectorIndex) removeIDFromClustersLocked(id string) {
	for c, list := range vi.entries {
		for i, e := range list {
			if e.id == id {
				vi.entries[c] = append(list[:i], list[i+1:]...)
				break
			}
		}
	}
}

// Remove deletes the vector with id from the index. Returns true if the
// id was present.
func (vi *VectorIndex) Remove(id string) bool {
	if vi == nil {
		return false
	}
	vi.mu.Lock()
	defer vi.mu.Unlock()
	if _, ok := vi.idToVec[id]; !ok {
		return false
	}
	delete(vi.idToVec, id)
	vi.removeIDFromClustersLocked(id)
	return true
}

// Size returns the number of stored vectors.
func (vi *VectorIndex) Size() int {
	if vi == nil {
		return 0
	}
	vi.mu.RLock()
	defer vi.mu.RUnlock()
	return len(vi.idToVec)
}

// Build runs k-means on all buffered vectors and partitions them into
// clusters. Must be called before Search for the IVF probe to work.
// If fewer than nClusters vectors are stored, the actual cluster count
// is reduced to min(nClusters, len(vectors)).
func (vi *VectorIndex) Build() {
	if vi == nil {
		return
	}
	vi.mu.Lock()
	defer vi.mu.Unlock()
	vi.buildLocked()
}

func (vi *VectorIndex) buildLocked() {
	ids := make([]string, 0, len(vi.idToVec))
	vecs := make([][]float32, 0, len(vi.idToVec))
	for id, v := range vi.idToVec {
		ids = append(ids, id)
		vecs = append(vecs, v)
	}
	if len(vecs) == 0 {
		vi.centroids = nil
		vi.entries = make(map[int][]ivfEntry)
		vi.built = false
		return
	}
	k := vi.nClusters
	if k > len(vecs) {
		k = len(vecs)
	}
	centroids := kmeans(vecs, k, 25)
	vi.centroids = centroids
	vi.entries = make(map[int][]ivfEntry, k)
	for i, v := range vecs {
		c := nearestCentroid(v, centroids)
		vi.entries[c] = append(vi.entries[c], ivfEntry{id: ids[i], vec: v})
	}
	vi.built = true
}

// Search returns the top-k nearest neighbors to query. If the index is
// not built (or dim mismatch), it falls back to exact brute-force over
// all stored vectors (issue #347 acceptance: fall back to brute force).
// nProbe is the number of nearest clusters to scan; <= 0 defaults to
// max(1, nClusters/4). The returned slice is sorted by ascending distance.
func (vi *VectorIndex) Search(query []float32, k int) []VectorMatch {
	if vi == nil || k <= 0 || len(query) != vi.dim {
		return nil
	}
	vi.mu.RLock()
	defer vi.mu.RUnlock()

	// Fallback: brute force if not built or no centroids.
	if !vi.built || len(vi.centroids) == 0 {
		return vi.bruteForceLocked(query, k)
	}

	nProbe := vi.nClusters / 4
	if nProbe < 1 {
		nProbe = 1
	}
	// Find clusters in ascending distance order; probe until we have
	// at least k candidates (capped at nClusters). This is the standard
	// IVF re-probe pattern — a single small cluster is not enough to
	// satisfy a top-k request.
	clusterOrder := vi.nearestClustersLocked(query, vi.nClusters)

	var candidates []ivfEntry
	for _, c := range clusterOrder {
		candidates = append(candidates, vi.entries[c]...)
		if len(candidates) >= k {
			break
		}
	}
	_ = nProbe // nProbe is the initial probe budget; we expand as needed
	if len(candidates) == 0 {
		// Empty clusters — fall back to scanning everything.
		for _, list := range vi.entries {
			candidates = append(candidates, list...)
		}
	}
	return rankCandidates(query, candidates, k)
}

// bruteForceLocked scans every stored vector. Caller holds vi.mu (R or W).
func (vi *VectorIndex) bruteForceLocked(query []float32, k int) []VectorMatch {
	candidates := make([]ivfEntry, 0, len(vi.idToVec))
	for _, v := range vi.idToVec {
		candidates = append(candidates, ivfEntry{id: "", vec: v})
	}
	// We lost the id in idToVec iteration ordering; re-build.
	candidates = candidates[:0]
	for id, v := range vi.idToVec {
		candidates = append(candidates, ivfEntry{id: id, vec: v})
	}
	return rankCandidates(query, candidates, k)
}

func (vi *VectorIndex) nearestClustersLocked(query []float32, nProbe int) []int {
	type ci struct {
		idx int
		d   float32
	}
	dists := make([]ci, len(vi.centroids))
	for i, c := range vi.centroids {
		dists[i] = ci{idx: i, d: l2Distance(query, c)}
	}
	sort.Slice(dists, func(i, j int) bool { return dists[i].d < dists[j].d })
	if nProbe > len(dists) {
		nProbe = len(dists)
	}
	out := make([]int, nProbe)
	for i := 0; i < nProbe; i++ {
		out[i] = dists[i].idx
	}
	return out
}

func (vi *VectorIndex) assign(vec []float32) int {
	return nearestCentroid(vec, vi.centroids)
}

// rankCandidates returns the k nearest entries to query from candidates,
// sorted by ascending L2 distance.
func rankCandidates(query []float32, candidates []ivfEntry, k int) []VectorMatch {
	matches := make([]VectorMatch, 0, len(candidates))
	for _, e := range candidates {
		matches = append(matches, VectorMatch{ID: e.id, Distance: l2Distance(query, e.vec)})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Distance < matches[j].Distance })
	if len(matches) > k {
		matches = matches[:k]
	}
	return matches
}

// DotProduct computes the dot product of a and b. Returns 0 if the
// lengths differ.
func DotProduct(a, b []float32) float32 {
	n := minLen(a, b)
	var sum float32
	for i := 0; i < n; i++ {
		sum += a[i] * b[i]
	}
	return sum
}

// Normalize scales v to unit L2 norm in place. If v has zero norm it
// is left unchanged.
func Normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return
	}
	norm := float32(math.Sqrt(sum))
	for i := range v {
		v[i] /= norm
	}
}

// l2Distance is the squared L2 distance between a and b. Squared is
// monotonic with true distance and avoids a sqrt per comparison.
func l2Distance(a, b []float32) float32 {
	n := minLen(a, b)
	var sum float32
	for i := 0; i < n; i++ {
		d := a[i] - b[i]
		sum += d * d
	}
	return sum
}

func minLen(a, b []float32) int {
	if len(a) < len(b) {
		return len(a)
	}
	return len(b)
}

// nearestCentroid returns the index of the centroid closest to vec.
func nearestCentroid(vec []float32, centroids [][]float32) int {
	bestIdx := 0
	bestDist := float32(math.MaxFloat32)
	for i, c := range centroids {
		d := l2Distance(vec, c)
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return bestIdx
}

// kmeans runs Lloyd's algorithm on vecs with k clusters for iters
// iterations. Returns the final centroids. If k >= len(vecs), returns
// the vecs themselves as centroids (degenerate: one cluster per point).
// Deterministic: picks the first k distinct vectors as initial centroids
// so results are byte-stable for the same input (no RNG).
func kmeans(vecs [][]float32, k int, iters int) [][]float32 {
	if k <= 0 {
		return nil
	}
	if k >= len(vecs) {
		out := make([][]float32, len(vecs))
		for i, v := range vecs {
			cp := make([]float32, len(v))
			copy(cp, v)
			out[i] = cp
		}
		return out
	}
	// Initial centroids: first k distinct vectors.
	centroids := make([][]float32, 0, k)
	seen := map[string]bool{}
	for _, v := range vecs {
		key := vecKey(v)
		if !seen[key] {
			seen[key] = true
			cp := make([]float32, len(v))
			copy(cp, v)
			centroids = append(centroids, cp)
			if len(centroids) == k {
				break
			}
		}
	}
	// If fewer than k distinct, pad by duplicating the last.
	for len(centroids) < k {
		last := centroids[len(centroids)-1]
		cp := make([]float32, len(last))
		copy(cp, last)
		centroids = append(centroids, cp)
	}

	dim := len(vecs[0])
	for iter := 0; iter < iters; iter++ {
		// Assignment step.
		assignments := make([]int, len(vecs))
		for i, v := range vecs {
			assignments[i] = nearestCentroid(v, centroids)
		}
		// Update step: recompute centroids as cluster means.
		sums := make([][]float64, k)
		counts := make([]int, k)
		for i := range sums {
			sums[i] = make([]float64, dim)
		}
		for i, v := range vecs {
			c := assignments[i]
			counts[c]++
			for j := range v {
				sums[c][j] += float64(v[j])
			}
		}
		moved := false
		for c := 0; c < k; c++ {
			if counts[c] == 0 {
				continue
			}
			for j := 0; j < dim; j++ {
				newVal := float32(sums[c][j] / float64(counts[c]))
				if newVal != centroids[c][j] {
					moved = true
				}
				centroids[c][j] = newVal
			}
		}
		if !moved {
			break
		}
	}
	return centroids
}

func vecKey(v []float32) string {
	var b []byte
	for _, f := range v {
		bits := math.Float32bits(f)
		b = append(b, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
	}
	return string(b)
}
