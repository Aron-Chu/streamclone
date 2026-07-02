package main

import (
	"os"
	"testing"
)

func TestShouldStartPublicCacheRefreshForThisProcess(t *testing.T) {
	tests := []struct {
		name string
		env  *string
		want bool
	}{
		{name: "unset defaults to api process", want: true},
		{name: "api process explicit zero", env: strPtr("0"), want: true},
		{name: "api process explicit false", env: strPtr("false"), want: true},
		{name: "worker process one", env: strPtr("1"), want: false},
		{name: "worker process true", env: strPtr("true"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env == nil {
				unsetenvForTest(t, "CORPUS_WORKERS_ENABLED")
			} else {
				t.Setenv("CORPUS_WORKERS_ENABLED", *tt.env)
			}
			if got := shouldStartPublicCacheRefreshForThisProcess(); got != tt.want {
				t.Fatalf("shouldStartPublicCacheRefreshForThisProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func unsetenvForTest(t *testing.T, key string) {
	t.Helper()
	original, hadOriginal := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadOriginal {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func strPtr(v string) *string {
	return &v
}
