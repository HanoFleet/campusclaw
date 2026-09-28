package retrieval

import "sort"

const rrfK = 60

type rankedItem struct {
	ID    int64
	Score float64
	Rank  int
}

func rrfMerge(lists [][]rankedItem) []rankedItem {
	type acc struct {
		score float64
		seen  map[int]bool
	}
	merged := map[int64]*acc{}
	for listIdx, list := range lists {
		for _, item := range list {
			if item.Rank <= 0 {
				continue
			}
			a := merged[item.ID]
			if a == nil {
				a = &acc{seen: map[int]bool{}}
				merged[item.ID] = a
			}
			if a.seen[listIdx] {
				continue
			}
			a.seen[listIdx] = true
			a.score += 1.0 / float64(rrfK+item.Rank)
		}
	}
	out := make([]rankedItem, 0, len(merged))
	for id, a := range merged {
		out = append(out, rankedItem{ID: id, Score: a.score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}
