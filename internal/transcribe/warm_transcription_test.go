package transcribe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"vocis/internal/config"
)

func TestEnsureTranscribeModelLoaded_NoopForOpenAI(t *testing.T) {
	t.Parallel()

	called := false
	err := EnsureTranscribeModelLoaded(context.Background(), config.TranscriptionConfig{
		Backend: config.BackendOpenAI,
		Model:   "gpt-4o-mini-transcribe",
		BaseURL: "http://this-must-never-be-called.invalid",
	}, func(string) {
		called = true
	})
	if err != nil {
		t.Fatalf("want nil error for openai backend, got %v", err)
	}
	if called {
		t.Fatalf("onLoading must not fire for the openai backend")
	}
}

func TestEnsureTranscribeModelLoaded_NoopWhenAlreadyLoaded(t *testing.T) {
	t.Parallel()

	stub := newLemonadeStub(t, stubSpec{loaded: []string{"whisper-v3-turbo-FLM"}})
	defer stub.Close()

	onLoadingCount := 0
	err := EnsureTranscribeModelLoaded(context.Background(), config.TranscriptionConfig{
		Backend: config.BackendLemonade,
		Model:   "whisper-v3-turbo-FLM",
		BaseURL: stub.URL + "/api/v1",
	}, func(string) { onLoadingCount++ })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if onLoadingCount != 0 {
		t.Fatalf("onLoading fired %d times, want 0 when model already loaded", onLoadingCount)
	}
	if got := atomic.LoadInt32(&stub.warmRequests); got != 0 {
		t.Fatalf("warm POST count = %d, want 0 when model already loaded", got)
	}
}

func TestEnsureTranscribeModelLoaded_LoadsWhenMissing(t *testing.T) {
	t.Parallel()

	stub := newLemonadeStub(t, stubSpec{loaded: []string{"gemma4-it-e2b-FLM"}})
	defer stub.Close()

	var gotModel string
	onLoadingCount := 0
	err := EnsureTranscribeModelLoaded(context.Background(), config.TranscriptionConfig{
		Backend: config.BackendLemonade,
		Model:   "whisper-v3-turbo-FLM",
		BaseURL: stub.URL + "/api/v1",
	}, func(m string) {
		onLoadingCount++
		gotModel = m
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if onLoadingCount != 1 {
		t.Fatalf("onLoading fired %d times, want 1", onLoadingCount)
	}
	if gotModel != "whisper-v3-turbo-FLM" {
		t.Fatalf("onLoading model = %q, want whisper-v3-turbo-FLM", gotModel)
	}
	if got := atomic.LoadInt32(&stub.warmRequests); got != 1 {
		t.Fatalf("warm POST count = %d, want 1", got)
	}
}

func TestEnsureTranscribeModelLoaded_PropagatesWarmError(t *testing.T) {
	t.Parallel()

	stub := newLemonadeStub(t, stubSpec{
		loaded:     []string{"gemma4-it-e2b-FLM"},
		warmStatus: http.StatusInternalServerError,
		warmBody:   "oom: cannot fit whisper-v3-turbo",
	})
	defer stub.Close()

	err := EnsureTranscribeModelLoaded(context.Background(), config.TranscriptionConfig{
		Backend: config.BackendLemonade,
		Model:   "whisper-v3-turbo-FLM",
		BaseURL: stub.URL + "/api/v1",
	}, nil)
	if err == nil {
		t.Fatalf("want error from warm failure, got nil")
	}
	if !strings.Contains(err.Error(), "whisper-v3-turbo") {
		t.Fatalf("error %q should mention the model name", err.Error())
	}
}

type stubSpec struct {
	loaded     []string
	warmStatus int
	warmBody   string
}

type lemonadeStub struct {
	*httptest.Server
	warmRequests int32
}

// newLemonadeStub fakes the subset of the Lemonade REST API the
// preflight touches: GET /api/v1/health for the loaded-models list
// and POST /api/v1/audio/transcriptions for the warm.
func newLemonadeStub(t *testing.T, spec stubSpec) *lemonadeStub {
	t.Helper()

	state := &lemonadeStub{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		loaded := make([]LemonadeLoadedModel, 0, len(spec.loaded))
		for _, name := range spec.loaded {
			loaded = append(loaded, LemonadeLoadedModel{Name: name, Type: "audio"})
		}
		body := map[string]any{
			"status":            "ok",
			"all_models_loaded": loaded,
			"max_models":        map[string]int{"audio": 1},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/api/v1/audio/transcriptions", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&state.warmRequests, 1)
		if spec.warmStatus != 0 && spec.warmStatus != http.StatusOK {
			w.WriteHeader(spec.warmStatus)
			_, _ = w.Write([]byte(spec.warmBody))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":""}`))
	})
	state.Server = httptest.NewServer(mux)
	return state
}
