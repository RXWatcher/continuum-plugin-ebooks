package runtime

import "testing"

func TestIntFrom(t *testing.T) {
	cases := []struct {
		in   any
		want int
		ok   bool
	}{
		{float64(10), 10, true},
		{int(4), 4, true},
		{int64(7), 7, true},
		{"10", 10, true},   // portal sometimes serializes numbers as strings
		{"  12 ", 12, true},
		{"abc", 0, false},
		{nil, 0, false},
		{true, 0, false},
	}
	for _, c := range cases {
		got, ok := intFrom(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("intFrom(%#v) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestMarshalJSONValue(t *testing.T) {
	if _, ok := marshalJSONValue(nil); ok {
		t.Error("nil value must not produce JSON (would become \"null\")")
	}
	b, ok := marshalJSONValue(map[string]any{"host": "smtp", "port": 587})
	if !ok || len(b) == 0 {
		t.Fatalf("valid object should marshal, got ok=%v b=%q", ok, b)
	}
	if string(b) == "null" {
		t.Errorf("must never emit the literal null, got %q", b)
	}
}
