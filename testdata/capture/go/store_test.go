package store

import "testing"

func TestGrade(t *testing.T) {
	cases := map[int]string{95: "A", 85: "B", 50: "F"}
	for score, want := range cases {
		t.Run(want, func(t *testing.T) {
			if got := Grade(score); got != want {
				t.Errorf("Grade(%d) = %s, want %s", score, got, want)
			}
		})
	}
}

func TestGradeBoundary(t *testing.T) {
	if got := Grade(90); got != "A" {
		t.Errorf("Grade(90) = %s, want A", got)
	}
}

func TestFindUser(t *testing.T) {
	t.Skip("needs a database")
}
