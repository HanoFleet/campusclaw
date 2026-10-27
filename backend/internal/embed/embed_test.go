package embed

import "testing"

func TestDimFor(t *testing.T) {
	cases := []struct {
		url, model string
		want       int
	}{
		{"local", "local-ngram-256", LocalDim},
		{"", "course-embedding", LocalDim},
		{"https://ai-gateway.devops.hello1023.com/v1", "course-embedding", GatewayDim},
		{"https://ai-gateway.devops.hello1023.com/v1", "local-ngram-256", LocalDim},
	}
	for _, c := range cases {
		if got := DimFor(c.url, c.model); got != c.want {
			t.Fatalf("DimFor(%q, %q)=%d, want %d", c.url, c.model, got, c.want)
		}
	}
}

func TestNewHTTPUsesGatewayDim(t *testing.T) {
	h := NewHTTP("https://ai-gateway.devops.hello1023.com/v1", "k", "course-embedding")
	if h.Dim() != GatewayDim {
		t.Fatalf("Dim()=%d", h.Dim())
	}
}

func TestLocalEmbedDim(t *testing.T) {
	vecs, err := Local{}.Embed(t.Context(), []string{"去括号"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 1 || len(vecs[0]) != LocalDim {
		t.Fatalf("got %d vectors dim %d", len(vecs), len(vecs[0]))
	}
}
