package interpreter

import "testing"

func TestEvalCondition(t *testing.T) {
	fact := Fact{"new_set": true, "name": "Strixhaven", "count": float64(3)}

	cases := []struct {
		cond string
		want bool
	}{
		{"new_set == true", true},
		{"new_set == false", false},
		{"new_set != false", true},
		{`name == "Strixhaven"`, true},
		{`name == "Other"`, false},
		{"count == 3", true},
		{"count != 3", false},
		{"missing == true", false}, // absent key compares unequal
		{"missing != true", true},
	}
	for _, c := range cases {
		got, err := evalCondition(c.cond, fact)
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.cond, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %v want %v", c.cond, got, c.want)
		}
	}
}

func TestEvalCondition_Unsupported(t *testing.T) {
	if _, err := evalCondition("new_set", Fact{}); err == nil {
		t.Error("expected error for a condition with no operator")
	}
}
