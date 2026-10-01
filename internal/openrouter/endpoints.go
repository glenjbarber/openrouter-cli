package openrouter

import (
	"context"
	"encoding/json"
	"strconv"
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

// UnmarshalJSON decodes a model, tolerating a price the endpoint quotes as a
// number rather than as a string.
//
// The price is carried as a string so that a figure written in exponent form
// survives intact, which is the form the endpoint uses. A catalogue is decoded
// as a whole, so one model quoted differently would otherwise fail the entire
// list and leave the listing with nothing to show. A price that is not a
// figure at all is carried as the text it arrived as, so that Free can refuse
// it rather than have to see an absent field.
func (m *Model) UnmarshalJSON(b []byte) error {
	var raw struct {
		ID            string          `json:"id"`
		Name          string          `json:"name"`
		Description   string          `json:"description"`
		ContextLength json.RawMessage `json:"context_length"`
		Prompt        json.RawMessage `json:"prompt"`
		Completion    json.RawMessage `json:"completion"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	m.ID = raw.ID
	m.Name = raw.Name
	m.Description = raw.Description
	m.ContextLength = tokenCount(raw.ContextLength)
	m.PromptPrice = scalarText(raw.Prompt)
	m.CompletionPrice = scalarText(raw.Completion)
	return nil
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
		return nil, c.wrapf(err, "decoding the model list")
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
		return nil, c.wrapf(err, "decoding the key usage")
	}
	return &envelope.Data, nil
}

// Free reports whether the model costs nothing to call.
//
// A price is quoted as a decimal string, so a model is free when both prices are
// zero. A price that cannot be read is not treated as free, since a model whose
// cost is unknown is not one to spend an allowance on by accident.
func (m Model) Free() bool {
	zero := func(s string) bool {
		if s == "" {
			// A model that quotes no price is not assumed to be free. The
			// endpoint omits the field for a model it does not price, and
			// assuming otherwise would list a paid model as free.
			return false
		}
		v, err := strconv.ParseFloat(s, 64)
		return err == nil && v == 0
	}
	return zero(m.PromptPrice) && zero(m.CompletionPrice)
}
