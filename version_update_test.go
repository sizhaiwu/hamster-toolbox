package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReleaseVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		current, latest, state string
		available, development bool
	}{
		{"0.2.2-dev", "v0.2.1", "development", false, true},
		{"0.2.2-dev", "v0.2.2", "available", true, true},
		{"0.2.1", "v0.2.1", "current", false, false},
		{"0.2.1+build.5", "v0.2.1", "current", false, false},
		{"v0.2.1", "v0.2.10", "available", true, false},
		{"0.2.10", "v0.2.9", "ahead", false, false},
		{"dev", "v1.0.0", "development", false, true},
		{"1.999999999999999999999999.1", "v1.2.9999", "ahead", false, false},
	} {
		got := describeVersionUpdate(tc.current, releaseSnapshot{tag: tc.latest, state: "ok"}, false)
		if got.State != tc.state || got.Available != tc.available || got.Development != tc.development {
			t.Fatalf("%+v: %+v", tc, got)
		}
	}
	for _, tag := range []string{"v1.0.0-rc.1", "1.2", "garbage", "v01.2.3", "v1.0.0<script>"} {
		if _, ok := compareReleaseVersion("0.1.0", tag); ok {
			t.Fatal("invalid stable tag", tag)
		}
	}
}
func testReleaseChecker(t *testing.T, handler http.HandlerFunc) *releaseChecker {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return &releaseChecker{client: &http.Client{Timeout: time.Second}, endpoint: ts.URL + "/releases/latest"}
}
func TestReleaseCheckerCacheConcurrencyAndRefresh(t *testing.T) {
	var calls atomic.Int32
	c := testReleaseChecker(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" || r.Header.Get("User-Agent") != "HamsterToolbox-UpdateCheck" {
			t.Error("unexpected local data or credentials in public request")
		}
		time.Sleep(15 * time.Millisecond)
		w.Write([]byte(`{"tag_name":"v0.3.0","html_url":"https://evil.example/","draft":false,"prerelease":false}`))
	})
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := c.check(context.Background(), "0.2.2-dev", false)
			if err != nil || !got.Available || got.URL != releasePageURL {
				t.Error(got, err)
			}
		}()
	}
	wait.Wait()
	if calls.Load() != 1 {
		t.Fatal("parallel windows repeated network calls", calls.Load())
	}
	if _, err := c.check(context.Background(), "0.2.2-dev", true); err != nil || calls.Load() != 1 {
		t.Fatal("manual refresh ignored cooldown")
	}
	c.mu.Lock()
	c.cached.checked = time.Now().Add(-time.Minute)
	c.mu.Unlock()
	if _, err := c.check(context.Background(), "0.2.2-dev", true); err != nil || calls.Load() != 2 {
		t.Fatal("manual refresh never refetched")
	}
	c.mu.Lock()
	c.cached.expires = time.Now().Add(-time.Second)
	c.mu.Unlock()
	if _, err := c.check(context.Background(), "0.2.2-dev", false); err != nil || calls.Load() != 3 {
		t.Fatal("expired cache never refetched")
	}
}
func TestReleaseCheckerFailureStates(t *testing.T) {
	for _, tc := range []struct {
		name, body, state string
		code              int
	}{
		{"unpublished", "", "unpublished", 404},
		{"rate limit", `{"message":"rate limit"}`, "unavailable", 403},
		{"offline", "", "unavailable", 503},
		{"bad JSON", `{`, "unavailable", 200},
		{"multiple JSON", `{"tag_name":"v0.3.0"}{}`, "unavailable", 200},
		{"draft", `{"tag_name":"v0.3.0","draft":true}`, "unavailable", 200},
		{"prerelease", `{"tag_name":"v0.3.0","prerelease":true}`, "unavailable", 200},
		{"unmarked prerelease", `{"tag_name":"v0.3.0-beta"}`, "unavailable", 200},
		{"unsafe tag", `{"tag_name":"https://evil.example/"}`, "unavailable", 200},
		{"oversized", `{"body":"` + strings.Repeat("a", (1<<20)+1) + `","tag_name":"v0.3.0"}`, "unavailable", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testReleaseChecker(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); w.Write([]byte(tc.body)) })
			got, err := c.check(context.Background(), "0.2.1", false)
			if err != nil || got.State != tc.state || got.Available || got.Latest != "" || got.URL != releasePageURL {
				t.Fatal(got, err)
			}
		})
	}
}
func TestReleaseCheckerCancellationAndTimeout(t *testing.T) {
	c := testReleaseChecker(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c.client.Timeout = 70 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.check(ctx, "0.2.1", false); err == nil {
		t.Fatal("cancelled page ignored")
	}
	got, err := c.check(context.Background(), "0.2.1", false)
	if err != nil || got.State != "unavailable" || got.Available {
		t.Fatal("timeout reported latest", got, err)
	}
}
func TestVersionUpdateAPIProtection(t *testing.T) {
	var calls atomic.Int32
	c := testReleaseChecker(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.Write([]byte(`{"tag_name":"v0.2.1"}`)) })
	s := &server{token: "release-test", updates: c}
	r := httptest.NewRequest("POST", "/api/version-update", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.guard(s.api)(w, r)
	if w.Code != 403 || calls.Load() != 0 {
		t.Fatal("anonymous update access")
	}
	for _, body := range []string{`{"URL":"https://evil.example"}`, `{} {}`, `{"Force":"true"}`} {
		r = httptest.NewRequest("POST", "/api/version-update", strings.NewReader(body))
		r.Header.Set("X-Toolbox-Token", s.token)
		w = httptest.NewRecorder()
		s.guard(s.api)(w, r)
		if w.Code != 400 || calls.Load() != 0 {
			t.Fatal("invalid request accepted")
		}
	}
	w = requestAPI(t, s, "/api/version-update", map[string]bool{"Force": false})
	var result versionUpdate
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Latest != "v0.2.1" || calls.Load() != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
}
