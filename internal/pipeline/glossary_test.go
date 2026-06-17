package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oukeidos/focst-local/internal/glossary"
	"github.com/oukeidos/focst-local/internal/llamaserver"
	"github.com/oukeidos/focst-local/internal/postpolish"
	"github.com/oukeidos/focst-local/internal/recovery"
	"github.com/oukeidos/focst-local/internal/translation"
)

func TestMergeMappingsNamesOverrideGlossary(t *testing.T) {
	got := mergeMappings(
		map[string]string{
			"架空田一郎": "가공다 이치로",
			"合成都市":  "합성 도시",
		},
		map[string]string{
			"架空田一郎": "수동 이치로",
		},
	)
	if got["架空田一郎"] != "수동 이치로" {
		t.Fatalf("names override did not win: %+v", got)
	}
	if got["合成都市"] != "합성 도시" {
		t.Fatalf("glossary-only mapping missing: %+v", got)
	}
}

func TestMappingOverrideCount(t *testing.T) {
	got := mappingOverrideCount(
		map[string]string{
			"架空田一郎": "가공다 이치로",
			"合成都市":  "합성 도시",
		},
		map[string]string{
			"架空田一郎": "수동 이치로",
			"仮想海":   "가상 바다",
		},
	)
	if got != 1 {
		t.Fatalf("mappingOverrideCount = %d, want 1", got)
	}
}

func TestRunTranslationAutoGlossaryInjectsWithoutSavingByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := writeSyntheticSRT(t, tmpDir)
	outputPath := filepath.Join(tmpDir, "out.srt")
	defaultGlossaryPath := filepath.Join(tmpDir, "out.glossary.json")
	defaultArtifactsDir := filepath.Join(tmpDir, "out.glossary")
	server := newPipelineGlossaryTestServer(t)
	defer server.Close()

	result, err := RunTranslation(context.Background(), Config{
		InputPath:            inputPath,
		OutputPath:           outputPath,
		BaseURL:              server.URL + "/v1",
		Model:                "test-model",
		MaxTokens:            8192,
		TranslationTimeout:   time.Second,
		ChunkSize:            2,
		ContextSize:          0,
		Concurrency:          1,
		SentenceAwareChunks:  false,
		ChunkBoundaryPlanner: ChunkBoundaryPlannerOff,
		NoPreprocess:         true,
		NoPostprocess:        true,
		SourceLang:           "ja",
		TargetLang:           "ko",
		Overwrite:            true,
		AutoGlossary:         true,
		GlossaryRuns:         1,
		GlossaryWindowChunks: 3,
		LlamaManager:         staticLlamaManager{},
	})
	if err != nil {
		t.Fatalf("RunTranslation failed: %v", err)
	}
	if result.Status != TranslationStatusSuccess {
		t.Fatalf("status = %s, want success", result.Status)
	}
	server.assertCounts(t, 1, 1, 1)
	if !server.systemPromptContains("- 架空田一郎 -> 가공다 이치로") {
		t.Fatalf("translation system prompt did not include generated glossary mapping:\n%s", server.lastTranslationSystemPrompt())
	}
	if _, err := os.Stat(defaultGlossaryPath); !os.IsNotExist(err) {
		t.Fatalf("default glossary artifact should not be saved, stat err = %v", err)
	}
	if _, err := os.Stat(defaultArtifactsDir); !os.IsNotExist(err) {
		t.Fatalf("default glossary debug dir should not be created, stat err = %v", err)
	}
}

func TestRunTranslationAutoGlossarySavesAndInjectsWhenSavePathSet(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := writeSyntheticSRT(t, tmpDir)
	outputPath := filepath.Join(tmpDir, "out.srt")
	glossaryPath := filepath.Join(tmpDir, "out.glossary.json")
	artifactsDir := filepath.Join(tmpDir, "out.glossary")
	server := newPipelineGlossaryTestServer(t)
	defer server.Close()

	result, err := RunTranslation(context.Background(), Config{
		InputPath:            inputPath,
		OutputPath:           outputPath,
		BaseURL:              server.URL + "/v1",
		Model:                "test-model",
		MaxTokens:            8192,
		TranslationTimeout:   time.Second,
		ChunkSize:            2,
		ContextSize:          0,
		Concurrency:          1,
		SentenceAwareChunks:  false,
		ChunkBoundaryPlanner: ChunkBoundaryPlannerOff,
		NoPreprocess:         true,
		NoPostprocess:        true,
		SourceLang:           "ja",
		TargetLang:           "ko",
		Overwrite:            true,
		AutoGlossary:         true,
		SaveGlossaryPath:     glossaryPath,
		GlossaryArtifactsDir: artifactsDir,
		GlossaryRuns:         1,
		GlossaryWindowChunks: 3,
		LlamaManager:         staticLlamaManager{},
	})
	if err != nil {
		t.Fatalf("RunTranslation failed: %v", err)
	}
	if result.Status != TranslationStatusSuccess {
		t.Fatalf("status = %s, want success", result.Status)
	}
	server.assertCounts(t, 1, 1, 1)
	if !server.systemPromptContains("- 架空田一郎 -> 가공다 이치로") {
		t.Fatalf("translation system prompt did not include generated glossary mapping:\n%s", server.lastTranslationSystemPrompt())
	}
	artifact, err := glossary.LoadArtifact(glossaryPath)
	if err != nil {
		t.Fatalf("generated glossary not loadable: %v", err)
	}
	if len(artifact.Entries) != 1 || artifact.Entries[0].Source != "架空田一郎" {
		t.Fatalf("unexpected generated artifact entries: %+v", artifact.Entries)
	}
	if artifact.RenderingSafetyFilter == nil || !artifact.RenderingSafetyFilter.Applied {
		t.Fatalf("generated artifact missing rendering safety filter metadata: %+v", artifact.RenderingSafetyFilter)
	}
	if _, err := os.Stat(filepath.Join(artifactsDir, "window_000", "run_001_prompt.txt")); err != nil {
		t.Fatalf("expected debug prompt artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifactsDir, "rendering_safety_filter", "batch_001_prompt.txt")); err != nil {
		t.Fatalf("expected rendering safety filter artifact: %v", err)
	}
	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if !strings.Contains(string(out), "번역 架空田一郎です") {
		t.Fatalf("output missing translated synthetic text:\n%s", string(out))
	}
}

func TestRunTranslationGlossaryFileReusesArtifactWithoutExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := writeSyntheticSRT(t, tmpDir)
	outputPath := filepath.Join(tmpDir, "out.srt")
	glossaryPath := filepath.Join(tmpDir, "existing.glossary.json")
	if err := glossary.SaveArtifact(glossaryPath, glossary.Artifact{
		Version:       1,
		PromptVersion: glossary.PromptVersion,
		SourceLang:    "ja",
		TargetLang:    "ko",
		Input: glossary.InputInfo{
			Path:                     "synthetic.srt",
			PreprocessedSegmentCount: 2,
			SegmentsChecksum:         "sha256:test",
		},
		Config: glossary.RunConfig{
			Model:                "test-model",
			BaseURL:              "http://127.0.0.1:8080/v1",
			MaxTokens:            8192,
			GlossaryRuns:         1,
			GlossaryWindowChunks: 3,
		},
		Entries: []glossary.Entry{{
			Source:        "架空田一郎",
			Rendering:     "기존 이치로",
			Confidence:    glossary.ConfidenceHigh,
			Votes:         map[string]int{"기존 이치로": 1},
			OccurrenceIDs: []int{1},
			WindowsSeen:   []int{0},
		}},
	}); err != nil {
		t.Fatalf("failed to save fixture glossary: %v", err)
	}
	server := newPipelineGlossaryTestServer(t)
	defer server.Close()

	result, err := RunTranslation(context.Background(), Config{
		InputPath:            inputPath,
		OutputPath:           outputPath,
		BaseURL:              server.URL + "/v1",
		Model:                "test-model",
		MaxTokens:            8192,
		TranslationTimeout:   time.Second,
		ChunkSize:            2,
		ContextSize:          0,
		Concurrency:          1,
		SentenceAwareChunks:  false,
		ChunkBoundaryPlanner: ChunkBoundaryPlannerOff,
		NoPreprocess:         true,
		NoPostprocess:        true,
		SourceLang:           "ja",
		TargetLang:           "ko",
		Overwrite:            true,
		GlossaryPath:         glossaryPath,
		LlamaManager:         staticLlamaManager{},
	})
	if err != nil {
		t.Fatalf("RunTranslation failed: %v", err)
	}
	if result.Status != TranslationStatusSuccess {
		t.Fatalf("status = %s, want success", result.Status)
	}
	server.assertCounts(t, 0, 0, 1)
	if !server.systemPromptContains("- 架空田一郎 -> 기존 이치로") {
		t.Fatalf("translation system prompt did not include reused glossary mapping:\n%s", server.lastTranslationSystemPrompt())
	}
}

func TestRunTranslationPostPolishWorksWithoutGlossaryOrNames(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := writeSyntheticSRT(t, tmpDir)
	outputPath := filepath.Join(tmpDir, "out.srt")
	correctionsPath := filepath.Join(tmpDir, "out.polish.json")
	server := newPipelineGlossaryTestServer(t)
	defer server.Close()

	result, err := RunTranslation(context.Background(), Config{
		InputPath:                 inputPath,
		OutputPath:                outputPath,
		BaseURL:                   server.URL + "/v1",
		Model:                     "test-model",
		MaxTokens:                 8192,
		TranslationTimeout:        time.Second,
		ChunkSize:                 2,
		ContextSize:               0,
		Concurrency:               1,
		SentenceAwareChunks:       false,
		ChunkBoundaryPlanner:      ChunkBoundaryPlannerOff,
		NoPreprocess:              true,
		NoPostprocess:             true,
		SourceLang:                "ja",
		TargetLang:                "ko",
		Overwrite:                 true,
		PostPolish:                true,
		SavePolishCorrectionsPath: correctionsPath,
		PolishBroadChunkSize:      30,
		PolishRepairChunkSize:     100,
		PolishMaxTokens:           2048,
		LlamaManager:              staticLlamaManager{},
	})
	if err != nil {
		t.Fatalf("RunTranslation failed: %v", err)
	}
	if result.Status != TranslationStatusSuccess {
		t.Fatalf("status = %s, want success", result.Status)
	}
	server.assertCounts(t, 0, 0, 1)
	server.assertPostPolishCount(t, 2)
	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if !strings.Contains(string(out), "교정 架空田一郎입니다") {
		t.Fatalf("output missing post-polish correction:\n%s", string(out))
	}
	var artifact postpolish.Artifact
	data, err := os.ReadFile(correctionsPath)
	if err != nil {
		t.Fatalf("failed to read polish artifact: %v", err)
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("failed to parse polish artifact: %v", err)
	}
	if len(artifact.Accepted) != 1 || artifact.Accepted[0].Pass != postpolish.PassRepair {
		t.Fatalf("unexpected polish artifact: %+v", artifact)
	}
}

func TestRunGlossaryExtractionWritesArtifactWithoutTranslation(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := writeSyntheticSRT(t, tmpDir)
	glossaryPath := filepath.Join(tmpDir, "extract.glossary.json")
	artifactsDir := filepath.Join(tmpDir, "extract.glossary")
	server := newPipelineGlossaryTestServer(t)
	defer server.Close()

	result, err := RunGlossaryExtraction(context.Background(), Config{
		InputPath:            inputPath,
		OutputPath:           glossaryPath,
		BaseURL:              server.URL + "/v1",
		Model:                "test-model",
		MaxTokens:            8192,
		TranslationTimeout:   time.Second,
		ChunkSize:            2,
		ContextSize:          0,
		Concurrency:          1,
		SentenceAwareChunks:  false,
		ChunkBoundaryPlanner: ChunkBoundaryPlannerOff,
		NoPreprocess:         true,
		SourceLang:           "ja",
		TargetLang:           "ko",
		Overwrite:            true,
		GlossaryArtifactsDir: artifactsDir,
		GlossaryRuns:         1,
		GlossaryWindowChunks: 3,
		LlamaManager:         staticLlamaManager{},
	})
	if err != nil {
		t.Fatalf("RunGlossaryExtraction failed: %v", err)
	}
	server.assertCounts(t, 1, 1, 0)
	if result.OutputPath != glossaryPath {
		t.Fatalf("output path = %q, want %q", result.OutputPath, glossaryPath)
	}
	artifact, err := glossary.LoadArtifact(glossaryPath)
	if err != nil {
		t.Fatalf("extracted glossary not loadable: %v", err)
	}
	if len(artifact.Entries) != 1 || artifact.Entries[0].Source != "架空田一郎" {
		t.Fatalf("unexpected extracted artifact entries: %+v", artifact.Entries)
	}
	if artifact.RenderingSafetyFilter == nil || !artifact.RenderingSafetyFilter.Applied {
		t.Fatalf("extracted artifact missing rendering safety filter metadata: %+v", artifact.RenderingSafetyFilter)
	}
	if _, err := os.Stat(filepath.Join(artifactsDir, "merged_glossary.json")); err != nil {
		t.Fatalf("expected merged debug artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifactsDir, "merged_glossary.prefilter.json")); err != nil {
		t.Fatalf("expected prefilter debug artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifactsDir, "rendering_safety_filter", "judgments.json")); err != nil {
		t.Fatalf("expected rendering safety filter judgments artifact: %v", err)
	}
}

func TestResolveRuntimeSessionLogFailsWhenGlossaryArtifactMissing(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "synthetic.srt")
	if err := os.WriteFile(inputPath, []byte("1\n00:00:01,000 --> 00:00:02,000\n架空田一郎です\n"), 0600); err != nil {
		t.Fatalf("failed to write input: %v", err)
	}
	logPath := filepath.Join(tmpDir, "out_recovery.json")
	_, err := resolveRuntimeSessionLog(logPath, &recovery.SessionLog{
		InputPath:    "synthetic.srt",
		GlossaryPath: "missing.glossary.json",
	})
	if err == nil || !strings.Contains(err.Error(), "glossary_path not found") {
		t.Fatalf("expected missing glossary artifact error, got %v", err)
	}
}

type staticLlamaManager struct{}

func (staticLlamaManager) Ensure(_ context.Context, cfg llamaserver.LaunchConfig) (llamaserver.ManagedServer, func(context.Context) error, error) {
	return llamaserver.ManagedServer{
		BaseURL: cfg.BaseURL,
		Config:  cfg,
	}, func(context.Context) error { return nil }, nil
}

func (staticLlamaManager) Status(context.Context, string) (llamaserver.Status, error) {
	return llamaserver.Status{}, nil
}

func (staticLlamaManager) Stop(context.Context, llamaserver.LockFile) error {
	return nil
}

type pipelineGlossaryTestServer struct {
	*httptest.Server
	mu                       sync.Mutex
	glossaryRequests         int
	renderingSafetyRequests  int
	translationRequests      int
	postPolishRequests       int
	translationSystemPrompts []string
}

func newPipelineGlossaryTestServer(t *testing.T) *pipelineGlossaryTestServer {
	t.Helper()
	state := &pipelineGlossaryTestServer{}
	state.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		responseFormat, _ := payload["response_format"].(map[string]any)
		formatType, _ := responseFormat["type"].(string)
		messages, _ := payload["messages"].([]any)
		systemPrompt := messageContent(messages, 0)
		userPrompt := messageContent(messages, 1)

		w.Header().Set("Content-Type", "application/json")
		switch formatType {
		case "text":
			switch systemPrompt {
			case glossary.SystemPrompt:
				state.mu.Lock()
				state.glossaryRequests++
				state.mu.Unlock()
				writeChatContent(t, w, "| Source | Korean rendering |\n| --- | --- |\n| 架空田一郎 | 가공다 이치로 |\n", 5, 7)
			case glossary.RenderingSafetyFilterSystemPrompt:
				state.mu.Lock()
				state.renderingSafetyRequests++
				state.mu.Unlock()
				writeChatContent(t, w, "| Row | Expected strategy | Fit | Decision |\n| --- | --- | --- | --- |\n| 1 | name_form | fits | keep |\n", 3, 4)
			default:
				http.Error(w, "unexpected text system prompt", http.StatusBadRequest)
			}
		case "json_object":
			if strings.Contains(systemPrompt, "subtitle copyeditor") {
				state.mu.Lock()
				state.postPolishRequests++
				state.mu.Unlock()
				resp := postpolish.Response{Corrections: []postpolish.ResponseCorrection{{
					ID:         1,
					SourceText: "架空田一郎です",
					Text:       "교정 架空田一郎입니다",
				}}}
				content, err := json.Marshal(resp)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeChatContent(t, w, string(content), 2, 3)
				return
			}
			state.mu.Lock()
			state.translationRequests++
			state.translationSystemPrompts = append(state.translationSystemPrompts, systemPrompt)
			state.mu.Unlock()
			var req translation.RequestData
			if err := json.Unmarshal([]byte(userPrompt), &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			resp := translation.ResponseData{}
			for _, segment := range req.Target {
				resp.Translations = append(resp.Translations, translation.TranslatedSegment{
					ID:         segment.ID,
					SourceText: segment.SourceText,
					Text:       "번역 " + segment.SourceText,
				})
			}
			content, err := json.Marshal(resp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeChatContent(t, w, string(content), 11, 13)
		default:
			http.Error(w, "unexpected response_format", http.StatusBadRequest)
		}
	}))
	return state
}

func (s *pipelineGlossaryTestServer) assertCounts(t *testing.T, glossaryRequests, renderingSafetyRequests, translationRequests int) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.glossaryRequests != glossaryRequests || s.renderingSafetyRequests != renderingSafetyRequests || s.translationRequests != translationRequests {
		t.Fatalf("requests = glossary:%d rendering_safety:%d translation:%d, want glossary:%d rendering_safety:%d translation:%d", s.glossaryRequests, s.renderingSafetyRequests, s.translationRequests, glossaryRequests, renderingSafetyRequests, translationRequests)
	}
}

func (s *pipelineGlossaryTestServer) assertPostPolishCount(t *testing.T, postPolishRequests int) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.postPolishRequests != postPolishRequests {
		t.Fatalf("post-polish requests = %d, want %d", s.postPolishRequests, postPolishRequests)
	}
}

func (s *pipelineGlossaryTestServer) systemPromptContains(needle string) bool {
	return strings.Contains(s.lastTranslationSystemPrompt(), needle)
}

func (s *pipelineGlossaryTestServer) lastTranslationSystemPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.translationSystemPrompts) == 0 {
		return ""
	}
	return s.translationSystemPrompts[len(s.translationSystemPrompts)-1]
}

func messageContent(messages []any, index int) string {
	if index >= len(messages) {
		return ""
	}
	msg, _ := messages[index].(map[string]any)
	content, _ := msg["content"].(string)
	return content
}

func writeChatContent(t *testing.T, w http.ResponseWriter, content string, promptTokens, completionTokens int) {
	t.Helper()
	body := map[string]any{
		"choices": []map[string]any{{
			"message": map[string]any{
				"role":    "assistant",
				"content": content,
			},
		}},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("failed to write response: %v", err)
	}
}

func writeSyntheticSRT(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "synthetic.srt")
	content := `1
00:00:01,000 --> 00:00:02,000
架空田一郎です

2
00:00:03,000 --> 00:00:04,000
合成都市へ行く
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write fixture srt: %v", err)
	}
	return path
}
