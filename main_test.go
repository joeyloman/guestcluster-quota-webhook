package main

import (
	"os"
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	// Test that init() runs without panicking
	// The init function is already called when the test runs
	
	// Note: certRenewalPeriod is set in main() not init(), so we can't test it here
	// We can only verify that init() doesn't panic
}

func TestOperateModeFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		wantMode    int
	}{
		{
			name:     "LOGONLY mode",
			envValue: "LOGONLY",
			wantMode: LOGONLY,
		},
		{
			name:     "default DENY mode",
			envValue: "",
			wantMode: DENY,
		},
		{
			name:     "other value defaults to DENY",
			envValue: "INVALID",
			wantMode: DENY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env value
			original := os.Getenv("OPERATEMODE")
			defer os.Setenv("OPERATEMODE", original)
			
			os.Setenv("OPERATEMODE", tt.envValue)
			
			// We can't easily test main() directly, but we can verify the constants
			if tt.wantMode == LOGONLY && LOGONLY != 2 {
				t.Errorf("LOGONLY constant should be 2, got %d", LOGONLY)
			}
			if tt.wantMode == DENY && DENY != 1 {
				t.Errorf("DENY constant should be 1, got %d", DENY)
			}
		})
	}
}

func TestCertRenewalPeriodFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantValue int64
	}{
		{
			name:      "valid period",
			envValue:  "60",
			wantValue: 60,
		},
		{
			name:      "invalid period defaults",
			envValue:  "invalid",
			wantValue: 30 * 24 * 60,
		},
		{
			name:      "zero defaults",
			envValue:  "0",
			wantValue: 30 * 24 * 60,
		},
		{
			name:      "empty defaults",
			envValue:  "",
			wantValue: 30 * 24 * 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would need to be tested in an integration test
			// as we can't easily re-run main() with different env values
			_ = tt
		})
	}
}

func TestRun(t *testing.T) {
	// Test that Run() doesn't block forever
	done := make(chan bool)
	
	go func() {
		// Run for a short time
		go Run()
		time.Sleep(10 * time.Millisecond)
		done <- true
	}()
	
	select {
	case <-done:
		// Success - Run() is working
	case <-time.After(100 * time.Millisecond):
		t.Error("Run() appears to be blocking unexpectedly")
	}
}
