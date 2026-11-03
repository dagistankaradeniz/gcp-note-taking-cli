package canvasrender

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"sort"
	"testing"
)

// The shared Canvas v1 fixtures: the CLI must normalise exactly like web,
// iOS and Android (same comparison rule as the web's contract.test.ts).

type fixtures struct {
	ElementCases []struct {
		Name     string         `json:"name"`
		Input    any            `json:"input"`
		Expected map[string]any `json:"expected"`
	} `json:"elementCases"`
	ListCases []struct {
		Name        string   `json:"name"`
		Input       []any    `json:"input"`
		ExpectedIDs []string `json:"expectedIds"`
	} `json:"listCases"`
}

func same(a, b any) bool {
	if fa, ok := a.(float64); ok {
		fb, ok := b.(float64)
		return ok && math.Abs(fa-fb) <= 1e-6*math.Max(1, math.Max(math.Abs(fa), math.Abs(fb)))
	}
	switch av := a.(type) {
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !same(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			w, ok := bv[k]
			if !ok || !same(v, w) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

// roundTrip turns typed values ([]any of maps, float64s) into the same
// shapes JSON decoding produces, so they compare with the fixtures.
func roundTrip(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func loadFixtures(t *testing.T) fixtures {
	t.Helper()
	raw, err := os.ReadFile("testdata/canvas_contract_fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestContractElementFixtures(t *testing.T) {
	f := loadFixtures(t)
	if len(f.ElementCases) == 0 {
		t.Fatal("no element cases loaded")
	}
	for _, c := range f.ElementCases {
		got := NormalizeElement(c.Input)
		if c.Expected == nil {
			if got != nil {
				t.Errorf("%s: expected drop, got %v", c.Name, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("%s: dropped, expected %v", c.Name, c.Expected)
			continue
		}
		if actual := roundTrip(t, got); !same(actual, any(c.Expected)) {
			gotJSON, _ := json.Marshal(actual)
			t.Errorf("%s:\n got  %s", c.Name, gotJSON)
		}
	}
}

func TestContractListFixtures(t *testing.T) {
	for _, c := range loadFixtures(t).ListCases {
		var ids []string
		for _, el := range NormalizeElements(c.Input) {
			ids = append(ids, el["id"].(string))
		}
		if !reflect.DeepEqual(ids, c.ExpectedIDs) && !(len(ids) == 0 && len(c.ExpectedIDs) == 0) {
			sort.Strings(ids)
			t.Errorf("%s: got %v want %v", c.Name, ids, c.ExpectedIDs)
		}
	}
}
