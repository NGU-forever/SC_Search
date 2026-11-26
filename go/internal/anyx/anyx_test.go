package anyx

import (
	"reflect"
	"testing"
)

func TestString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"", ""},
		{" hi ", " hi "},
		{[]byte("bytes"), ""},
		{42, ""},
		{int64(42), ""},
		{3.14, ""},
		{true, ""},
		{[]string{"a"}, ""},
	}
	for _, c := range cases {
		if got := String(c.in); got != c.want {
			t.Errorf("String(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBool(t *testing.T) {
	if !Bool(true) || Bool(false) || Bool("true") || Bool(nil) || Bool(1) {
		t.Fatalf("unexpected Bool results")
	}
}

func TestFloat(t *testing.T) {
	cases := []struct {
		in   any
		want float64
	}{
		{3.5, 3.5},
		{int64(2), 2},
		{3, 3},
		{"4", 0},
		{nil, 0},
		{true, 0},
		{[]byte("1"), 0},
	}
	for _, c := range cases {
		if got := Float(c.in); got != c.want {
			t.Errorf("Float(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestInt64(t *testing.T) {
	cases := []struct {
		in   any
		want int64
	}{
		{int64(7), 7},
		{8, 8},
		{9.9, 9},
		{-1.9, -1},
		{"10", 0},
		{nil, 0},
		{true, 0},
	}
	for _, c := range cases {
		if got := Int64(c.in); got != c.want {
			t.Errorf("Int64(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMaps(t *testing.T) {
	a := map[string]any{"a": 1}
	b := map[string]any{"b": 2}

	if got := Maps(nil); got != nil {
		t.Errorf("Maps(nil) = %#v, want nil", got)
	}
	if got := Maps("x"); got != nil {
		t.Errorf("Maps(string) = %#v, want nil", got)
	}
	direct := []map[string]any{a}
	if got := Maps(direct); !reflect.DeepEqual(got, direct) {
		t.Errorf("Maps([]map) = %#v, want %#v", got, direct)
	}
	got := Maps([]any{a, "skip", nil, b})
	if !reflect.DeepEqual(got, []map[string]any{a, b}) {
		t.Errorf("Maps([]any) = %#v", got)
	}
	empty := Maps([]any{})
	if empty == nil || len(empty) != 0 {
		t.Errorf("Maps(empty []any) = %#v, want non-nil empty", empty)
	}
}

func TestStringMap(t *testing.T) {
	m := map[string]any{"k": "v"}
	if got := StringMap(m); !reflect.DeepEqual(got, m) {
		t.Errorf("StringMap(map) = %#v", got)
	}
	for _, in := range []any{nil, "x", []any{m}, map[string]string{"k": "v"}} {
		if got := StringMap(in); got != nil {
			t.Errorf("StringMap(%#v) = %#v, want nil", in, got)
		}
	}
}

func TestSlice(t *testing.T) {
	s := []any{1, "a"}
	if got := Slice(s); !reflect.DeepEqual(got, s) {
		t.Errorf("Slice([]any) = %#v", got)
	}
	for _, in := range []any{nil, "x", []string{"a"}, []map[string]any{{}}} {
		if got := Slice(in); got != nil {
			t.Errorf("Slice(%#v) = %#v, want nil", in, got)
		}
	}
}

func TestStrings(t *testing.T) {
	if got := Strings(nil); got != nil {
		t.Errorf("Strings(nil) = %#v, want nil", got)
	}
	if got := Strings("x"); got != nil {
		t.Errorf("Strings(string) = %#v, want nil", got)
	}
	direct := []string{"a", ""}
	if got := Strings(direct); !reflect.DeepEqual(got, direct) {
		t.Errorf("Strings([]string) = %#v, want %#v", got, direct)
	}
	got := Strings([]any{"a", 1, nil, "", "b"})
	if !reflect.DeepEqual(got, []string{"a", "", "b"}) {
		t.Errorf("Strings([]any) = %#v", got)
	}
	empty := Strings([]any{})
	if empty == nil || len(empty) != 0 {
		t.Errorf("Strings(empty []any) = %#v, want non-nil empty", empty)
	}
}

func TestFirst(t *testing.T) {
	if got := First(); got != "" {
		t.Errorf("First() = %q", got)
	}
	if got := First("", "", "x", "y"); got != "x" {
		t.Errorf("First = %q", got)
	}
	if got := First("  ", "y"); got != "  " {
		t.Errorf("First keeps whitespace = %q", got)
	}
}

func TestFirstField(t *testing.T) {
	row := map[string]any{
		"a":     "  ",
		"b":     "<nil>",
		"c":     " value ",
		"d":     "later",
		"num":   7,
		"empty": "",
	}
	if got := FirstField(row, "a", "b", "c", "d"); got != "value" {
		t.Errorf("FirstField = %q, want value", got)
	}
	if got := FirstField(row, "missing", "num", "empty"); got != "" {
		t.Errorf("FirstField non-strings = %q, want empty", got)
	}
	if got := FirstField(nil, "x"); got != "" {
		t.Errorf("FirstField(nil) = %q", got)
	}
}

func TestUnique(t *testing.T) {
	got := Unique([]string{"b", "", "a", "b", "a", "c"})
	if !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Errorf("Unique = %#v", got)
	}
	if got := Unique(nil); got == nil || len(got) != 0 {
		t.Errorf("Unique(nil) = %#v, want non-nil empty", got)
	}
}

func TestAppendUnique(t *testing.T) {
	xs := []string{"a"}
	if got := AppendUnique(xs, ""); !reflect.DeepEqual(got, xs) {
		t.Errorf("AppendUnique empty = %#v", got)
	}
	if got := AppendUnique(xs, "a"); !reflect.DeepEqual(got, xs) {
		t.Errorf("AppendUnique dup = %#v", got)
	}
	if got := AppendUnique(xs, "b"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("AppendUnique new = %#v", got)
	}
	if got := AppendUnique(nil, "x"); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("AppendUnique nil = %#v", got)
	}
}
