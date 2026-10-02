package openrouter

import (
	"context"
	"encoding/json"
	"math"
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
		// Pricing is where the catalogue documents the prices, as an object
		// beside the other members of the model.
		Pricing struct {
			Prompt     json.RawMessage `json:"prompt"`
			Completion json.RawMessage `json:"completion"`
		} `json:"pricing"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	m.ID = raw.ID
	m.Name = raw.Name
	m.Description = raw.Description
	m.ContextLength = tokenCount(raw.ContextLength)
	// The prices are read from the pricing object when the model carries one,
	// and from the model itself otherwise, which is where the first reader of
	// the catalogue looked for them. The object wins, since it is the shape the
	// endpoint documents, and a member of it that is absent falls back to the
	// model's own so that a catalogue quoted either way is read.
	m.PromptPrice = scalarText(raw.Pricing.Prompt)
	if m.PromptPrice == "" {
		m.PromptPrice = scalarText(raw.Prompt)
	}
	m.CompletionPrice = scalarText(raw.Pricing.Completion)
	if m.CompletionPrice == "" {
		m.CompletionPrice = scalarText(raw.Completion)
	}
	return nil
}

// freeAllowance is the separate allowance the key endpoint reports for free
// models.
//
// It is an alias rather than a named type, so that the field carrying it holds
// the shape it has always held and a reader of the struct sees what it saw
// before.
type freeAllowance = struct {
	Used  float64 `json:"used"`
	Limit float64 `json:"limit"`
}

// Usage is what the key endpoint reports about a key.
type Usage struct {
	Usage float64 `json:"usage"`
	Limit float64 `json:"limit"`
	// FreeModelRequests is a separate allowance, absent when the key has no
	// free-model access.
	FreeModelRequests *freeAllowance `json:"free_model_daily_requests"`
}

// UnmarshalJSON decodes the allowance, reading a figure the endpoint quotes as
// text rather than as a number.
//
// The figures are carried as numbers because that is how the endpoint quotes
// them, but a figure written as a string would otherwise fail the whole decode
// and leave the reader with no allowance at all, which is a worse outcome than
// a figure that is read. The free allowance is optional, so one that is
// present but is not an object is left absent rather than failing the paid
// figures that were read correctly.
func (u *Usage) UnmarshalJSON(b []byte) error {
	var raw struct {
		Usage     json.RawMessage `json:"usage"`
		Limit     json.RawMessage `json:"limit"`
		FreeDaily json.RawMessage `json:"free_model_daily_requests"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	u.Usage = scalarFloat(raw.Usage)
	u.Limit = scalarFloat(raw.Limit)
	u.FreeModelRequests = nil
	if len(raw.FreeDaily) == 0 || string(raw.FreeDaily) == "null" {
		return nil
	}
	var free struct {
		Used  json.RawMessage `json:"used"`
		Limit json.RawMessage `json:"limit"`
	}
	if err := json.Unmarshal(raw.FreeDaily, &free); err != nil {
		return nil
	}
	u.FreeModelRequests = &freeAllowance{
		Used:  scalarFloat(free.Used),
		Limit: scalarFloat(free.Limit),
	}
	return nil
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

// Prices returns what the model charges for a token sent to it and for a token
// it sends back, in US dollars, and whether both are known.
//
// They are the figures the catalogue quotes, which are per token and not per
// million, so a caller wanting a cost multiplies them by a count. A price that
// is absent, unreadable, negative or not finite is not known, and neither is
// the pair when one of them is not. The endpoint quotes a negative price for a
// model whose cost depends on the route chosen, and a figure like that is not
// one to multiply a count by.
func (m Model) Prices() (prompt, completion float64, ok bool) {
	read := func(s string) (float64, bool) {
		if s == "" {
			return 0, false
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	}
	p, okP := read(m.PromptPrice)
	c, okC := read(m.CompletionPrice)
	if !okP || !okC {
		return 0, 0, false
	}
	return p, c, true
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
