package qdrant

import "testing"

func TestParseVectorSize(t *testing.T) {
	raw := `{
		"result": {
			"config": {
				"params": {
					"vectors": {"size": 2048, "distance": "Cosine"}
				}
			},
			"points_count": 3
		}
	}`
	size, ok := parseVectorSize(raw)
	if !ok || size != 2048 {
		t.Fatalf("size=%d ok=%v", size, ok)
	}
}
