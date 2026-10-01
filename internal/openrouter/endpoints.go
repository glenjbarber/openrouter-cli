package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
)

// Model is one model the endpoint offers.
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// ContextLength is the maximum context the model accepts, zero when the
	// endpoint does not report one.
	ContextLength int `json:"context_length"`
	// Pricing is quoted per token, so a caller wanting a cost divides by a
	// million. It is carried as a string because the endpoint reports it
	// that way and parsing it here would lose a value reported in
	// scientific notation.
	PromptPrice     string `json:"prompt"`
	CompletionPrice string `json:"completion"`
}

// Usage is what the key endpoint reports about a key.
type Usage struct {
	Usage float64 `json:"usage"`
	Limit float64 `json:"limit"`
	// FreeModelRequests is a separate allowance, absent when the key has no
	// free-model access.
	FreeModelRequests *struct {
		Used  float64 `json:"used"`
		Limit float64 `json:"limit"`
	} `json:"free_model_daily_requests"`
}

// Models returns the models the endpoint offers.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	data, err := c.do(ctx, "GET", "/models", nil)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decoding the model list: %w", err)
	}
	return envelope.Data, nil
}

// KeyUsage returns the usage and allowance recorded against the key.
func (c *Client) KeyUsage(ctx context.Context) (*Usage, error) {
	data, err := c.do(ctx, "GET", "/key", nil)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Data Usage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decoding the key usage: %w", err)
	}
	return &envelope.Data, nil
}
