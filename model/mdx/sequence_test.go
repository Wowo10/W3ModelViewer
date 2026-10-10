package mdx

import "testing"

func TestSequenceKey(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Stand - 1", "stand#1"},
		{"Stand -1", "stand#1"},
		{"Stand 1", "stand#1"},
		{"Walk", "walk#1"},
		{"Walk 1", "walk#1"},
		{"Attack - 2", "attack#2"},
		{"Attack 2", "attack#2"},
		{"Stand Ready", "stand ready#1"},
		{"Stand 01", "stand#1"},
		{"Bla 0", "bla#0"},
		{"", ""},
	}
	for _, c := range cases {
		if got := SequenceKey(c.name); got != c.want {
			t.Errorf("SequenceKey(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
