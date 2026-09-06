package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	freeAgentsSourceURL     = "https://raw.githubusercontent.com/CodebuffAI/codebuff/main/common/src/constants/free-agents.ts"
	freeModelsSourceURL     = "https://raw.githubusercontent.com/CodebuffAI/codebuff/main/common/src/constants/freebuff-models.ts"
	freeModelIDsSourceURL   = "https://raw.githubusercontent.com/CodebuffAI/codebuff/main/common/src/constants/freebuff-model-ids.ts"
	geminiSourceURL         = "https://raw.githubusercontent.com/CodebuffAI/codebuff/main/common/src/constants/gemini.ts"
	modelConfigSourceURL    = "https://raw.githubusercontent.com/CodebuffAI/codebuff/main/common/src/constants/model-config.ts"
	entitlementsSourceURL   = "https://raw.githubusercontent.com/CodebuffAI/codebuff/main/common/src/constants/freebuff-model-entitlements.ts"
	modelRefreshInterval    = 6 * time.Hour

	contextPrunerAgentID = "context-pruner"
	geminiParentAgentID  = "base2-free-deepseek-flash"
)

// isGeminiModel / geminiChatAgentForModel mirror free-buff-lol's dual run-chain
// routing: google/gemini-* models run as a subagent child of the deepseek-flash
// parent run.
func isGeminiModel(model string) bool {
	return strings.HasPrefix(model, "google/gemini-")
}

func geminiChatAgentForModel(model string) string {
	if strings.Contains(model, "pro") {
		return "thinker-with-files-gemini"
	}
	return "basher"
}

// modelAliases map short client-side names to canonical model ids.
var modelAliases = map[string]string{
	"deepseek-v4-pro":            "deepseek/deepseek-v4-pro",
	"deepseek-v4-flash":          "deepseek/deepseek-v4-flash",
	"deepseek-v3.1-terminus":     "deepseek/deepseek-v4-pro",
	"mimo-v2.5-pro":              "mimo/mimo-v2.5-pro",
	"mimo-v2.5":                  "mimo/mimo-v2.5",
	"kimi-k2.6":                  "moonshotai/kimi-k2.6",
	"kimi-k3-eco":                "crof/kimi-k3-eco",
	"minimax-m2.7":               "minimax/minimax-m2.7",
	"minimax-m3":                 "minimax/minimax-m3",
	"gemini-3.1-flash-lite":      "google/gemini-3.1-flash-lite",
	"gemini-3.5-flash-lite":      "google/gemini-3.5-flash-lite",
	"gemini-3.1-pro":             "google/gemini-3.1-pro-preview",
	"gemini-pro":                 "google/gemini-3.1-pro-preview",
	"gemini-3.8-flash":           "google/gemini-3.8-flash",
	"gpt-5.6-luna":               "openai/gpt-5.6-luna",
	"luna":                       "openai/gpt-5.6-luna",
	"solar-pro4":                 "upstage/solar-pro4",
	"glm-5.3-flash":              "z-ai/glm-5.3-flash",
	"claude-fable-5":             "anthropic/claude-fable-5",
	"fable-5":                    "anthropic/claude-fable-5",
}

// geminiHelperModels are not in the root-agent map; they run through the gemini
// dual run chain as helper subagents.
var geminiHelperModels = map[string]string{
	"google/gemini-2.5-flash-lite":         "file-picker",
	"google/gemini-3.1-flash-lite":         "basher",
	"google/gemini-3.1-flash-lite-preview": "basher",
	"google/gemini-3.5-flash-lite":         "basher",
	"google/gemini-3.1-pro-preview":        "thinker-with-files-gemini",
}

// localDenylist supplements the upstream paused list with models that are
// technically listed but effectively dead (0 req/h, perpetually broken, etc.).
var localDenylist = map[string]bool{
	"upstage/solar-pro4": true, // 0 req/h observed 2026-09
}

// hardcodedFallback is used when the remote fetch fails on startup. Values
// mirror the live free-agents.ts root map minus paused models (as of 2026-09).
var hardcodedFallback = map[string]string{
	"mimo/mimo-v2.5":                   "base2-free-mimo",
	"deepseek/deepseek-v4-flash":       "base2-free-deepseek-flash",
	"openai/gpt-5.6-luna":              "base2-free-luna",
	"openai/gpt-5.6-luna-es":           "base2-free-luna-es",
	"z-ai/glm-5.3-flash":               "base2-free-glm-5-3-flash",
	"crof/kimi-k3-eco":                 "base2-free-kimi-k3-eco",
	"anthropic/claude-fable-5":         "base2-free-fable",
	"google/gemini-3.8-flash":          "base2-free-gemini-3-8-flash",
	"meta/muse-spark-1.2-contributor":  "base2-free-muse-spark",
	"meta/muse-spark-1.3-contributor":  "base2-free-muse-spark-1-3",
}

// ModelRegistry resolves model→agent mappings from the upstream TypeScript
// constants: the FREEBUFF_ROOT_AGENT_ID_BY_MODEL map minus
// FREEBUFF_PAUSED_FREE_MODEL_IDS, with constant references resolved across
// source files.
type ModelRegistry struct {
	client *http.Client
	logger *log.Logger

	mu           sync.RWMutex
	modelToAgent map[string]string // model → agentID
	allModels    []string          // sorted
	lastOK       time.Time

	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewModelRegistry(client *http.Client, logger *log.Logger) *ModelRegistry {
	return &ModelRegistry{
		client:       client,
		logger:       logger,
		modelToAgent: make(map[string]string),
		stopCh:       make(chan struct{}),
	}
}

func (r *ModelRegistry) Start(ctx context.Context) {
	if err := r.refresh(ctx); err != nil {
		r.logger.Printf("model registry: initial fetch failed, loading hardcoded fallback: %v", err)
		r.loadFallback()
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(modelRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				if err := r.refresh(ctx); err != nil {
					r.logger.Printf("model registry: refresh failed: %v", err)
				}
				cancel()
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *ModelRegistry) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

func (r *ModelRegistry) Models() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.allModels))
	copy(out, r.allModels)
	return out
}

// CanonicalModel resolves aliases to a canonical model id.
func CanonicalModel(model string) string {
	model = strings.TrimSpace(model)
	if canonical, ok := modelAliases[model]; ok {
		return canonical
	}
	return model
}

func (r *ModelRegistry) HasModel(model string) bool {
	_, ok := r.AgentForModel(model)
	return ok
}

// AgentForModel returns the agent ID serving the given model (alias-resolved).
func (r *ModelRegistry) AgentForModel(model string) (string, bool) {
	model = CanonicalModel(model)
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.modelToAgent[model]
	return agent, ok
}

// AgentIDs returns distinct agent IDs for run prewarming.
func (r *ModelRegistry) AgentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]bool, len(r.modelToAgent))
	ids := make([]string, 0, len(r.modelToAgent))
	for _, agent := range r.modelToAgent {
		if !seen[agent] {
			seen[agent] = true
			ids = append(ids, agent)
		}
	}
	sort.Strings(ids)
	return ids
}

func (r *ModelRegistry) refresh(ctx context.Context) error {
	var sources []string
	for _, u := range []string{freeAgentsSourceURL, freeModelsSourceURL, freeModelIDsSourceURL, geminiSourceURL, modelConfigSourceURL, entitlementsSourceURL} {
		text, err := fetchSource(ctx, r.client, u)
		if err != nil {
			if u == freeAgentsSourceURL {
				return fmt.Errorf("fetch %s: %w", u, err)
			}
			r.logger.Printf("model registry: optional source %s failed: %v", u, err)
			continue
		}
		sources = append(sources, text)
	}

	consts := parseConstants(sources)
	rootMap := parseRootAgentMap(sources, consts)
	if len(rootMap) == 0 {
		return fmt.Errorf("no root agent mappings found in source")
	}
	paused := parsePausedModels(sources, consts)

	modelToAgent := make(map[string]string, len(rootMap)+len(geminiHelperModels))
	for model, agent := range rootMap {
		if _, isPaused := paused[model]; isPaused {
			continue
		}
		if localDenylist[model] {
			continue
		}
		modelToAgent[model] = agent
	}
	for model, agent := range geminiHelperModels {
		if _, isPaused := paused[model]; isPaused {
			continue
		}
		if localDenylist[model] {
			continue
		}
		if _, exists := modelToAgent[model]; !exists {
			modelToAgent[model] = agent
		}
	}

	allModels := make([]string, 0, len(modelToAgent))
	for model := range modelToAgent {
		allModels = append(allModels, model)
	}
	sort.Strings(allModels)

	r.mu.Lock()
	r.modelToAgent = modelToAgent
	r.allModels = allModels
	r.lastOK = time.Now()
	r.mu.Unlock()

	r.logger.Printf("model registry: updated %d models: %v", len(allModels), allModels)
	return nil
}

func (r *ModelRegistry) loadFallback() {
	modelToAgent := make(map[string]string, len(hardcodedFallback)+len(geminiHelperModels))
	for model, agent := range hardcodedFallback {
		modelToAgent[model] = agent
	}
	for model, agent := range geminiHelperModels {
		if _, exists := modelToAgent[model]; !exists {
			modelToAgent[model] = agent
		}
	}
	allModels := make([]string, 0, len(modelToAgent))
	for model := range modelToAgent {
		allModels = append(allModels, model)
	}
	sort.Strings(allModels)

	r.mu.Lock()
	r.modelToAgent = modelToAgent
	r.allModels = allModels
	r.mu.Unlock()

	r.logger.Printf("model registry: loaded fallback models: %v", allModels)
}

func fetchSource(ctx context.Context, client *http.Client, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch source: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	return stripTSComments(string(body)), nil
}

// stripTSComments removes // line comments and /* */ block comments so comment
// text (which is heavy in these files) cannot confuse the regex parsers.
func stripTSComments(source string) string {
	var out strings.Builder
	lines := strings.Split(source, "\n")
	inBlock := false
	for _, line := range lines {
		if inBlock {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				inBlock = false
				line = line[idx+2:]
			} else {
				continue
			}
		}
		for {
			idx := strings.Index(line, "/*")
			if idx < 0 {
				break
			}
			end := strings.Index(line[idx+2:], "*/")
			if end < 0 {
				line = line[:idx]
				inBlock = true
				break
			}
			line = line[:idx] + line[idx+2+end+2:]
		}
		if i := strings.Index(line, "//"); i >= 0 && !insideQuotes(line[:i]) {
			line = line[:i]
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

func insideQuotes(prefix string) bool {
	count := 0
	for i := 0; i < len(prefix); i++ {
		if prefix[i] == '\'' && (i == 0 || prefix[i-1] != '\\') {
			count++
		}
	}
	return count%2 == 1
}

var (
	constStringRe  = regexp.MustCompile(`(?m)^\s*export const ([A-Z][A-Z0-9_]+)(?:\s*:[^=\n]+)?\s*=\s*[\r\n\t ]*'([^']+)'`)
	constObjectRe  = regexp.MustCompile(`(?ms)^\s*export const ([A-Za-z][A-Za-z0-9_]+)(?:\s*:[^=\n]+)?\s*=\s*\{(.+?)\}`)
	objectKVRe     = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_]+)\s*:\s*'([^']+)'`)
	constRefRe     = regexp.MustCompile(`(?m)^\s*export const ([A-Z][A-Z0-9_]+)(?:\s*:[^=\n]+)?\s*=\s*([A-Za-z][A-Za-z0-9_]*)\.([A-Za-z][A-Za-z0-9_]*)\s*$`)
	rootMapEntryRe = regexp.MustCompile(`(?m)\[\s*('[^']+'|[A-Z][A-Z0-9_]+)\s*\]\s*:\s*('[^']+'|[A-Z][A-Z0-9_]+)`)
)

// parseConstants builds a name→string table from `export const NAME = 'value'`,
// object literals (`NAME = { key: 'value' }` → NAME.key), and member references
// (`NAME = OTHER.key`), iterating until stable.
func parseConstants(sources []string) map[string]string {
	consts := make(map[string]string)
	for _, source := range sources {
		for _, m := range constStringRe.FindAllStringSubmatch(source, -1) {
			consts[m[1]] = m[2]
		}
		for _, m := range constObjectRe.FindAllStringSubmatch(source, -1) {
			for _, kv := range objectKVRe.FindAllStringSubmatch(m[2], -1) {
				consts[m[1]+"."+kv[1]] = kv[2]
			}
		}
	}
	for pass := 0; pass < 5; pass++ {
		resolved := 0
		for _, source := range sources {
			for _, m := range constRefRe.FindAllStringSubmatch(source, -1) {
				if _, ok := consts[m[1]]; ok {
					continue
				}
				if value, ok := consts[m[2]+"."+m[3]]; ok {
					consts[m[1]] = value
					resolved++
				}
			}
		}
		if resolved == 0 {
			break
		}
	}
	return consts
}

func resolveConstant(token string, consts map[string]string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "'") && strings.HasSuffix(token, "'") && len(token) >= 2 {
		return token[1 : len(token)-1]
	}
	if value, ok := consts[token]; ok {
		return value
	}
	return ""
}

// parseRootAgentMap extracts FREEBUFF_ROOT_AGENT_ID_BY_MODEL entries.
func parseRootAgentMap(sources []string, consts map[string]string) map[string]string {
	result := make(map[string]string)
	for _, source := range sources {
		idx := strings.Index(source, "FREEBUFF_ROOT_AGENT_ID_BY_MODEL")
		if idx < 0 {
			continue
		}
		block := extractBalanced(source[idx:], '{', '}')
		if block == "" {
			continue
		}
		for _, m := range rootMapEntryRe.FindAllStringSubmatch(block, -1) {
			model := resolveConstant(m[1], consts)
			agent := resolveConstant(m[2], consts)
			if model != "" && agent != "" {
				result[model] = agent
			}
		}
	}
	return result
}

// parsePausedModels extracts FREEBUFF_PAUSED_FREE_MODEL_IDS entries.
func parsePausedModels(sources []string, consts map[string]string) map[string]bool {
	result := make(map[string]bool)
	for _, source := range sources {
		idx := strings.Index(source, "FREEBUFF_PAUSED_FREE_MODEL_IDS")
		if idx < 0 {
			continue
		}
		// Skip past the `=` so the `[` in the `readonly string[]` type
		// annotation isn't mistaken for the array literal.
		eq := strings.Index(source[idx:], "=")
		if eq < 0 {
			continue
		}
		block := extractBalanced(source[idx+eq:], '[', ']')
		if block == "" {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			token := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
			if token == "" || strings.HasPrefix(token, "export") || token == "[" || token == "]" {
				continue
			}
			if model := resolveConstant(token, consts); model != "" {
				result[model] = true
			}
		}
	}
	return result
}

// extractBalanced returns the substring from the first open bracket through its
// matching close bracket.
func extractBalanced(source string, open, close byte) string {
	start := strings.IndexByte(source, open)
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(source); i++ {
		switch source[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return source[start : i+1]
			}
		}
	}
	return ""
}
