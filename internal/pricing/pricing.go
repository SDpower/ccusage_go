package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Service struct {
	client    *http.Client
	cache     map[string]ModelPricing
	cacheMux  sync.RWMutex
	cacheTime time.Time
	cacheTTL  time.Duration
}

type ModelPricing struct {
	InputCostPerToken              float64 `json:"input_cost_per_token"`
	OutputCostPerToken             float64 `json:"output_cost_per_token"`
	CacheCreationInputTokenCost    float64 `json:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost        float64 `json:"cache_read_input_token_cost"`
	// server_tool_use per-request charges (Anthropic web tools).
	// LiteLLM does not yet publish these; loaded from embedded defaults.
	WebSearchCostPerRequest float64 `json:"web_search_cost_per_request,omitempty"`
	WebFetchCostPerRequest  float64 `json:"web_fetch_cost_per_request,omitempty"`
}

// AnthropicWebSearchCostPerRequest is Anthropic's published price for one
// web_search server-tool call. Update if Anthropic changes the rate.
const AnthropicWebSearchCostPerRequest = 0.01

// AnthropicWebFetchCostPerRequest is Anthropic's published price for one
// web_fetch server-tool call (currently unbilled; reserved for future use).
const AnthropicWebFetchCostPerRequest = 0.0

// LiteLLM uses direct model name mapping, not nested data structure
type LiteLLMResponse map[string]ModelPricing

func NewService() *Service {
	return &Service{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache:    make(map[string]ModelPricing),
		cacheTTL: 1 * time.Hour,
	}
}

func (s *Service) GetModelPrice(ctx context.Context, model string) (
	inputPrice, outputPrice, cacheCreatePrice, cacheReadPrice,
	webSearchPrice, webFetchPrice float64, err error,
) {
	s.cacheMux.RLock()
	if pricing, exists := s.cache[model]; exists && time.Since(s.cacheTime) < s.cacheTTL {
		s.cacheMux.RUnlock()
		return pricing.InputCostPerToken, pricing.OutputCostPerToken,
			pricing.CacheCreationInputTokenCost, pricing.CacheReadInputTokenCost,
			webPriceOrDefault(pricing.WebSearchCostPerRequest, AnthropicWebSearchCostPerRequest),
			webPriceOrDefault(pricing.WebFetchCostPerRequest, AnthropicWebFetchCostPerRequest),
			nil
	}
	s.cacheMux.RUnlock()

	if err := s.refreshCache(ctx); err != nil {
		return s.getEmbeddedPricing(model)
	}

	s.cacheMux.RLock()
	if pricing, exists := s.cache[model]; exists {
		s.cacheMux.RUnlock()
		return pricing.InputCostPerToken, pricing.OutputCostPerToken,
			pricing.CacheCreationInputTokenCost, pricing.CacheReadInputTokenCost,
			webPriceOrDefault(pricing.WebSearchCostPerRequest, AnthropicWebSearchCostPerRequest),
			webPriceOrDefault(pricing.WebFetchCostPerRequest, AnthropicWebFetchCostPerRequest),
			nil
	}
	s.cacheMux.RUnlock()

	return s.getEmbeddedPricing(model)
}

// webPriceOrDefault returns the configured price; if 0 (unconfigured / LiteLLM
// not yet publishing the field), use the Anthropic published default.
func webPriceOrDefault(configured, fallback float64) float64 {
	if configured > 0 {
		return configured
	}
	return fallback
}

func (s *Service) refreshCache(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json", nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var response LiteLLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}

	s.cacheMux.Lock()
	s.cache = response
	s.cacheTime = time.Now()
	s.cacheMux.Unlock()

	return nil
}

func (s *Service) getEmbeddedPricing(model string) (
	inputPrice, outputPrice, cacheCreatePrice, cacheReadPrice,
	webSearchPrice, webFetchPrice float64, err error,
) {
	// Embedded pricing for common models (per-token pricing matching TypeScript)
	embeddedPricing := map[string]ModelPricing{
		"claude-3-5-sonnet-20241022": {InputCostPerToken: 0.000003, OutputCostPerToken: 0.000015, CacheCreationInputTokenCost: 0.00000375, CacheReadInputTokenCost: 0.0000003},
		"claude-3-5-sonnet-20240620": {InputCostPerToken: 0.000003, OutputCostPerToken: 0.000015, CacheCreationInputTokenCost: 0.00000375, CacheReadInputTokenCost: 0.0000003},
		"claude-sonnet-4-5-20250929": {InputCostPerToken: 0.000003, OutputCostPerToken: 0.000015, CacheCreationInputTokenCost: 0.00000375, CacheReadInputTokenCost: 0.0000003},
		"claude-3-sonnet-20240229":   {InputCostPerToken: 0.000003, OutputCostPerToken: 0.000015, CacheCreationInputTokenCost: 0.00000375, CacheReadInputTokenCost: 0.0000003},
		"claude-3-haiku-20240307":    {InputCostPerToken: 0.00000025, OutputCostPerToken: 0.00000125, CacheCreationInputTokenCost: 0.0000003, CacheReadInputTokenCost: 0.00000003},
		"claude-haiku-4-5-20251001": {InputCostPerToken: 0.000001, OutputCostPerToken: 0.000005, CacheCreationInputTokenCost: 0.00000125, CacheReadInputTokenCost: 0.0000001},
		"claude-3-opus-20240229":     {InputCostPerToken: 0.000015, OutputCostPerToken: 0.000075, CacheCreationInputTokenCost: 0.01875, CacheReadInputTokenCost: 0.0000015},
		"gpt-4o":                     {InputCostPerToken: 0.000005, OutputCostPerToken: 0.000015, CacheCreationInputTokenCost: 0.0000125, CacheReadInputTokenCost: 0.0000005},
		"gpt-4o-mini":                {InputCostPerToken: 0.00000015, OutputCostPerToken: 0.0000006, CacheCreationInputTokenCost: 0.000000375, CacheReadInputTokenCost: 0.000000015},
		"gpt-4":                      {InputCostPerToken: 0.00003, OutputCostPerToken: 0.00006, CacheCreationInputTokenCost: 0.000075, CacheReadInputTokenCost: 0.000003},
		"gpt-3.5-turbo":              {InputCostPerToken: 0.0000005, OutputCostPerToken: 0.0000015, CacheCreationInputTokenCost: 0.00000125, CacheReadInputTokenCost: 0.00000005},
	}

	// Try to find exact match or with common prefixes/suffixes
	modelVariants := []string{
		model,
		"claude-3-5-" + model,
		"claude-3-" + model,
		"claude-" + model,
		model + "-20241022",
		model + "-20240620",
		model + "-20240229",
		model + "-20240307",
	}
	
	for _, variant := range modelVariants {
		if pricing, exists := embeddedPricing[variant]; exists {
			return pricing.InputCostPerToken, pricing.OutputCostPerToken,
				pricing.CacheCreationInputTokenCost, pricing.CacheReadInputTokenCost,
				webPriceOrDefault(pricing.WebSearchCostPerRequest, AnthropicWebSearchCostPerRequest),
				webPriceOrDefault(pricing.WebFetchCostPerRequest, AnthropicWebFetchCostPerRequest),
				nil
		}
	}

	// Default pricing for unknown models
	return 0.000001, 0.000002, 0.0000025, 0.0000001,
		AnthropicWebSearchCostPerRequest, AnthropicWebFetchCostPerRequest, nil
}
