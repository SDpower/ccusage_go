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
	InputPrice  float64 `json:"input_price"`
	OutputPrice float64 `json:"output_price"`
}

type LiteLLMResponse struct {
	Data map[string]ModelPricing `json:"data"`
}

func NewService() *Service {
	return &Service{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache:    make(map[string]ModelPricing),
		cacheTTL: 1 * time.Hour,
	}
}

func (s *Service) GetModelPrice(ctx context.Context, model string) (inputPrice, outputPrice float64, err error) {
	s.cacheMux.RLock()
	if pricing, exists := s.cache[model]; exists && time.Since(s.cacheTime) < s.cacheTTL {
		s.cacheMux.RUnlock()
		return pricing.InputPrice, pricing.OutputPrice, nil
	}
	s.cacheMux.RUnlock()

	// Try to refresh cache
	if err := s.refreshCache(ctx); err != nil {
		// Fall back to embedded pricing if API fails
		return s.getEmbeddedPricing(model)
	}

	s.cacheMux.RLock()
	if pricing, exists := s.cache[model]; exists {
		s.cacheMux.RUnlock()
		return pricing.InputPrice, pricing.OutputPrice, nil
	}
	s.cacheMux.RUnlock()

	// Model not found, return embedded pricing
	return s.getEmbeddedPricing(model)
}

func (s *Service) refreshCache(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://litellm-api.com/pricing", nil)
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
	s.cache = response.Data
	s.cacheTime = time.Now()
	s.cacheMux.Unlock()

	return nil
}

func (s *Service) getEmbeddedPricing(model string) (inputPrice, outputPrice float64, err error) {
	// Embedded pricing for common models (as fallback)
	embeddedPricing := map[string]ModelPricing{
		"claude-3-5-sonnet-20241022": {InputPrice: 3.0, OutputPrice: 15.0},
		"claude-3-5-sonnet-20240620": {InputPrice: 3.0, OutputPrice: 15.0},
		"claude-3-sonnet-20240229":   {InputPrice: 3.0, OutputPrice: 15.0},
		"claude-3-haiku-20240307":    {InputPrice: 0.25, OutputPrice: 1.25},
		"claude-3-opus-20240229":     {InputPrice: 15.0, OutputPrice: 75.0},
		"gpt-4o":                     {InputPrice: 5.0, OutputPrice: 15.0},
		"gpt-4o-mini":                {InputPrice: 0.15, OutputPrice: 0.6},
		"gpt-4":                      {InputPrice: 30.0, OutputPrice: 60.0},
		"gpt-3.5-turbo":              {InputPrice: 0.5, OutputPrice: 1.5},
	}

	if pricing, exists := embeddedPricing[model]; exists {
		return pricing.InputPrice, pricing.OutputPrice, nil
	}

	// Default pricing for unknown models
	return 1.0, 2.0, nil
}
