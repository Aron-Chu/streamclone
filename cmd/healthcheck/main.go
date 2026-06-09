package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	target := os.Getenv("HEALTHCHECK_URL")
	if target == "" {
		target = "http://localhost:8080/readyz"
	}
	if len(os.Args) > 1 {
		target = os.Args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "unhealthy status %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
