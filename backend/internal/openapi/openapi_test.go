package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type body struct {
	Name  string   `json:"name" required:"true" doc:"名称"`
	Tags  []string `json:"tags,omitempty"`
	Skip  string   `json:"-"`
	inner int
}

func TestHandleRegistersRouteAndDoc(t *testing.T) {
	mux := http.NewServeMux()
	d := Mount(mux, "t", "1", "")
	d.Handle("POST /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.PathValue("id")))
	}, Op{Summary: "改一项", Teacher: true, Body: body{}, Errors: map[int]string{404: "not_found"}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/items/7", nil))
	if rec.Body.String() != "7" {
		t.Fatalf("route not registered: %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var spec struct {
		Paths map[string]map[string]struct {
			Parameters []map[string]any `json:"parameters"`
			Security   []any            `json:"security"`
			Responses  map[string]any   `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	op, ok := spec.Paths["/items/{id}"]["post"]
	if !ok {
		t.Fatalf("operation missing from spec: %s", rec.Body.String())
	}
	if len(op.Parameters) != 1 || op.Parameters[0]["name"] != "id" || op.Parameters[0]["in"] != "path" {
		t.Fatalf("path param = %v", op.Parameters)
	}
	if len(op.Security) != 1 {
		t.Fatalf("teacher op should need bearer: %v", op.Security)
	}
	for _, code := range []string{"200", "401", "403", "404"} {
		if _, ok := op.Responses[code]; !ok {
			t.Fatalf("response %s missing: %v", code, op.Responses)
		}
	}
	if got := d.Paths(); !reflect.DeepEqual(got, []string{"POST /items/{id}"}) {
		t.Fatalf("Paths = %v", got)
	}
}

func TestSchemaFromStruct(t *testing.T) {
	s := Schema(body{})
	props := s["properties"].(map[string]any)
	if len(props) != 2 {
		t.Fatalf("properties = %v", props)
	}
	name := props["name"].(map[string]any)
	if name["type"] != "string" || name["description"] != "名称" {
		t.Fatalf("name = %v", name)
	}
	tags := props["tags"].(map[string]any)
	if tags["type"] != "array" {
		t.Fatalf("tags = %v", tags)
	}
	if !reflect.DeepEqual(s["required"], []string{"name"}) {
		t.Fatalf("required = %v", s["required"])
	}
}
