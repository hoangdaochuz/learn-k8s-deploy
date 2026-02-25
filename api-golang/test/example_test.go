package test

import "testing"

func TestOneEqualsOne(t *testing.T) {
	val := 1
	if val != 1 {
		t.Errorf("1 != %d; want 1", val)
	}
}

// Example test for testing cicd pipeline.
func TestTwoEqualsTwo(t *testing.T) {
	val := 2
	if val != 2 {
		t.Errorf("2 != %d; want 2", val)
	}
}
