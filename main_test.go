package main

import "testing"

func TestDefaultLayout(t *testing.T) {
	want := "g/a/v/a-v.jar"
	got := Gav{Group: "g", Artifact: "a", Version: "v"}.DefaultLayout()
	if want != got {
		t.Fatalf("Expected %s but got %s\n", want, got)
	}
}

func TestDefaultLayoutClassifier(t *testing.T) {
	want := "g/a/v/a-v-c.jar"
	gav := Gav{Group: "g", Artifact: "a", Version: "v", Classifier: "c"}
	got := gav.DefaultLayout()
	if want != got {
		t.Fatalf("Expected %s but got %s\n", want, got)
	}
}
