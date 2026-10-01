package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The endpoint is not under this repository's control, so every field that can
// legitimately be absent, null, or written in another shape has to be
// tolerated. A catalogue in particular is decoded as a whole, so one model
// quoted differently would otherwise fail the entire list and leave the
// listing with nothing to show.

// modelsFrom serves a catalogue and returns what the client decoded from it.
func modelsFrom(t *testing.T, body string) []Model {
	t.Helper()
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	return models
}

func TestModelsDecodeQuotedPrices(t *testing.T) {
	models := modelsFrom(t, `{"data":[{
		"id":"vendor/model",
		"name":"Model",
		"description":"A model.",
		"context_length":128000,
		"prompt":"0",
		"completion":"0"
	}]}`)
	if len(models) != 1 {
		t.Fatalf("len = %d, want 1", len(models))
	}
	m := models[0]
	if m.ID != "vendor/model" || m.Name != "Model" || m.Description != "A model." {
		t.Errorf("model = %+v, want the text fields", m)
	}
	if m.ContextLength != 128000 {
		t.Errorf("context = %d, want 128000", m.ContextLength)
	}
	if !m.Free() {
		t.Error("Free = false for a model quoted at zero twice")
	}
}

// A price quoted as a number rather than as a string is read as the figure it
// is, and does not take the rest of the catalogue down with it.
func TestModelsDecodeNumericPrices(t *testing.T) {
	models := modelsFrom(t, `{"data":[
		{"id":"a/free","prompt":0,"completion":0},
		{"id":"b/paid","prompt":0.0000015,"completion":0}
	]}`)
	if len(models) != 2 {
		t.Fatalf("len = %d, want 2", len(models))
	}
	if models[0].PromptPrice != "0" || models[0].CompletionPrice != "0" {
		t.Errorf("model = %+v, want the figures carried as text", models[0])
	}
	if !models[0].Free() {
		t.Error("Free = false for a model quoted at zero twice as numbers")
	}
	if models[1].Free() {
		t.Error("Free = true for a model charged for one direction")
	}
}

// A price that is null or absent is not free, since the endpoint omits the
// field for a model it does not price and assuming otherwise would spend an
// allowance on a paid model.
func TestModelsDecodeUnpricedAsUnpriced(t *testing.T) {
	models := modelsFrom(t, `{"data":[
		{"id":"a/null","prompt":null,"completion":"0"},
		{"id":"b/absent"},
		{"id":"c/junk","prompt":"free","completion":"0"},
		{"id":"d/object","prompt":{},"completion":"0"}
	]}`)
	if len(models) != 4 {
		t.Fatalf("len = %d, want 4", len(models))
	}
	for _, m := range models {
		if m.Free() {
			t.Errorf("Free = true for %s, quoted as %q and %q",
				m.ID, m.PromptPrice, m.CompletionPrice)
		}
	}
	// The text of a price that is not a figure is carried as it arrived, so
	// that the refusal above is a decision rather than an accident of the
	// field being empty.
	if models[2].PromptPrice != "free" {
		t.Errorf("prompt = %q, want the text kept", models[2].PromptPrice)
	}
	if models[3].PromptPrice != "{}" {
		t.Errorf("prompt = %q, want the text kept", models[3].PromptPrice)
	}
}

// A window that is absent is zero, which the caller reads as unknown rather
// than as a window of nothing. A window written as a string is read rather than
// refused.
func TestModelsDecodeTheWindowInEitherShape(t *testing.T) {
	models := modelsFrom(t, `{"data":[
		{"id":"a/absent"},
		{"id":"b/number","context_length":1000000},
		{"id":"c/string","context_length":"1000000"},
		{"id":"d/null","context_length":null},
		{"id":"e/junk","context_length":"many"}
	]}`)
	want := map[string]int{
		"a/absent": 0, "b/number": 1000000, "c/string": 1000000,
		"d/null": 0, "e/junk": 0,
	}
	if len(models) != len(want) {
		t.Fatalf("len = %d, want %d", len(models), len(want))
	}
	for _, m := range models {
		if m.ContextLength != want[m.ID] {
			t.Errorf("%s context = %d, want %d", m.ID, m.ContextLength, want[m.ID])
		}
	}
}

// A body that is not a catalogue is an error rather than an empty listing,
// since a listing with nothing in it and a listing that failed to load read
// the same on screen.
func TestModelsReportABodyThatIsNotACatalogue(t *testing.T) {
	for _, body := range []string{`[]`, `<html>maintenance</html>`, `{"data":"none"}`} {
		c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, body)
		})
		models, err := c.Models(context.Background())
		if err == nil {
			t.Errorf("Models(%s) = %+v, want an error", body, models)
		}
	}
}

// The allowance is a figure against a limit, not the conversation context, and
// a limit of zero is reported as zero rather than being turned into a share
// against nothing.
func TestKeyUsageReportsALimitOfZero(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"usage":12.5,"limit":0}}`)
	})
	usage, err := c.KeyUsage(context.Background())
	if err != nil {
		t.Fatalf("KeyUsage: %v", err)
	}
	if usage.Limit != 0 {
		t.Errorf("limit = %v, want the reported zero kept", usage.Limit)
	}
	if usage.Usage != 12.5 {
		t.Errorf("usage = %v, want 12.5", usage.Usage)
	}
	// The struct carries the figures. Dividing one by the other is the
	// caller decision, and a zero limit must be visible to it rather than
	// hidden behind a missing field.
	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("marshalling the usage: %v", err)
	}
	if !strings.Contains(string(data), `"limit":0`) {
		t.Errorf("usage = %s, want the zero limit reported", data)
	}
}

// A figure the endpoint leaves out is zero rather than a failure, and a
// missing data object is likewise not a failure.
func TestKeyUsageToleratesAbsentFields(t *testing.T) {
	for _, body := range []string{
		`{"data":{}}`,
		`{"data":{"usage":null,"limit":null,"free_model_daily_requests":null}}`,
		`{}`,
	} {
		c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, body)
		})
		usage, err := c.KeyUsage(context.Background())
		if err != nil {
			t.Errorf("KeyUsage(%s) = %v, want the absent figures tolerated", body, err)
			continue
		}
		if usage.Usage != 0 || usage.Limit != 0 {
			t.Errorf("KeyUsage(%s) = %+v, want zeroes", body, usage)
		}
		if usage.FreeModelRequests != nil {
			t.Errorf("KeyUsage(%s) = %+v, want no free allowance", body, usage.FreeModelRequests)
		}
	}
}

// A price the endpoint quotes with surrounding space is trimmed as it is
// decoded, so a model read from the catalogue never carries one. A figure
// handed to Free directly is read as written, since a malformed quote is not a
// price and is not free.
func TestFreeIsStrictAboutAHandBuiltPrice(t *testing.T) {
	m := Model{PromptPrice: " 0 ", CompletionPrice: "0"}
	if m.Free() {
		t.Error("Free = true for a price written with surrounding space")
	}
	if got := scalarText(json.RawMessage(`" 0 "`)); got != "0" {
		t.Errorf("scalarText = %q, want the surrounding space removed", got)
	}
}

// A free allowance is carried separately from the paid one, and a key that has
// one of them and not the other decodes.
func TestKeyUsageCarriesEitherAllowanceAlone(t *testing.T) {
	for _, body := range []string{
		`{"data":{"usage":1,"limit":2,"free_model_daily_requests":{"used":3,"limit":1000}}}`,
		`{"data":{"usage":1,"limit":2}}`,
	} {
		c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, body)
		})
		if _, err := c.KeyUsage(context.Background()); err != nil {
			t.Errorf("KeyUsage(%s) = %v, want it decoded", body, err)
		}
	}
}

// A model is free when both prices are zero, whatever notation the figure is
// written in, since a figure of zero in any notation is a figure of zero.
func TestFreeAcrossNotations(t *testing.T) {
	for _, prices := range [][2]string{
		{"0", "0"},
		{"0.0", "0.000000"},
		{"0e0", "0E-9"},
		{"-0", "0.0"},
	} {
		m := Model{PromptPrice: prices[0], CompletionPrice: prices[1]}
		if !m.Free() {
			t.Errorf("Free = false for %v, want a zero price to be zero", prices)
		}
	}
	// A charge of any size, however small, is a charge.
	for _, prices := range [][2]string{
		{"1e-30", "0"},
		{"0", "1e-30"},
		{"-1", "0"},
	} {
		m := Model{PromptPrice: prices[0], CompletionPrice: prices[1]}
		if m.Free() {
			t.Errorf("Free = true for %v, want a charged model refused", prices)
		}
	}
}

// A model priced in only one direction is not free, since the other direction
// is unpriced rather than free, and the record says an unpriced model is not
// one to spend an allowance on.
func TestFreeRefusesAHalfQuotedModel(t *testing.T) {
	if (Model{PromptPrice: "0"}).Free() {
		t.Error("Free = true for a model quoted in one direction only")
	}
	if (Model{CompletionPrice: "0"}).Free() {
		t.Error("Free = true for a model quoted in one direction only")
	}
}

// The credential is a header, and it is not sent as part of a body or a query.
func TestCredentialTravelsInAHeaderOnly(t *testing.T) {
	var raw string
	var query string
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		raw = string(body)
		query = r.URL.RawQuery
		io.WriteString(w, `{"data":[]}`)
	})

	if _, err := c.Models(context.Background()); err != nil {
		t.Fatalf("Models: %v", err)
	}
	if strings.Contains(raw, "test-key") || strings.Contains(query, "test-key") {
		t.Errorf("body = %q, query = %q, want the credential in neither", raw, query)
	}
}

// A count above what this build can hold is clamped to the bound this build
// actually has, rather than refused or refused against a figure written down for
// a 64-bit machine. A count that fits is delivered unchanged.
func TestTokenCountClampsToThePlatformBound(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int
	}{
		{`128000`, 128000},
		{`0`, 0},
		{`null`, 0},
		{`""`, 0},
		{``, 0},
		{`"not a figure"`, 0},
		{`"9223372036854775807"`, maxInt},
		{`1e30`, maxInt},
		{`99999999999999999999`, maxInt},
		{`-1e30`, minInt},
	} {
		if got := tokenCount(json.RawMessage(tc.raw)); got != tc.want {
			t.Errorf("tokenCount(%s) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}
