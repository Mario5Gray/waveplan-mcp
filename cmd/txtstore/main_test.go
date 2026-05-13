package main

import "testing"

func TestToolNameForWriteSwimPlan(t *testing.T) {
	got := toolNameForCommand("write-swim-plan")
	if got != "txtstore_write_swim_plan" {
		t.Fatalf("toolNameForCommand() = %q, want %q", got, "txtstore_write_swim_plan")
	}
}
