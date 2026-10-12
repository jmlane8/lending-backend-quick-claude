//go:build integration && load

package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jmlane8/lending-backend-quick-claude/tests/testutil"
)

// TestSubmitLatency checks the plan's performance goal: p95 < 300 ms for submissions
// (SC-003 requires < 1 s). Run with: go test -tags='integration load' -run Latency ./tests/contract/
func TestSubmitLatency(t *testing.T) {
	e := newEnv(t)
	token := e.signer.Token("load-tester", testutil.SubmitScope)

	post := func(i int) (time.Duration, int) {
		body := example(t, "single")
		set(body, []string{"customer", "idNumber"}, fmt.Sprintf("LOAD%06d", i))
		set(body, []string{"collateral", "licensePlate"}, fmt.Sprintf("L%06d", i))
		payload, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/loan-applications", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", uuid.NewString())
		req.Header.Set("Content-Type", "application/json")
		start := time.Now()
		resp, err := http.DefaultClient.Do(req)
		elapsed := time.Since(start)
		if err != nil {
			return elapsed, 0
		}
		resp.Body.Close()
		return elapsed, resp.StatusCode
	}

	var (
		mu        sync.Mutex
		latencies []time.Duration
		failures  int
	)
	record := func(d time.Duration, status int) {
		mu.Lock()
		defer mu.Unlock()
		latencies = append(latencies, d)
		if status != http.StatusCreated {
			failures++
		}
	}

	for i := range 500 {
		record(post(i))
	}
	var wg sync.WaitGroup
	for w := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 10 {
				record(post(1000 + w*10 + j))
			}
		}()
	}
	wg.Wait()

	slices.Sort(latencies)
	p95 := latencies[len(latencies)*95/100]
	p50 := latencies[len(latencies)/2]
	t.Logf("requests=%d p50=%v p95=%v max=%v", len(latencies), p50, p95, latencies[len(latencies)-1])
	if failures != 0 {
		t.Errorf("failures = %d, want 0", failures)
	}
	if p95 >= 300*time.Millisecond {
		t.Errorf("p95 = %v, want < 300ms", p95)
	}
}
