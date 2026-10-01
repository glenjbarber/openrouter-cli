package openrouter

import "encoding/json"

// ToolTypeFunction is the kind of tool the API names.
//
// It is a constant rather than a value a caller sets freely, because a kind
// this package does not know is a kind that would be sent as written and
// refused by the endpoint, with the refusal arriving as an error on a turn the
// caller believed was ordinary.
const ToolTypeFunction = "function"

// Tool is a tool offered to the model.
//
// It is a struct rather than a map, so that the wire names are spelled once here
// instead of at every call site, and so that a name spelled wrongly is refused
// by the compiler rather than by the endpoint.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction is the description of one tool the model may call.
//
// The parameters are carried unparsed. They are a JSON Schema document that the
// model reads, and decoding it here would buy nothing: a schema this package
// understood would be a schema it had to refuse when a tool needed a keyword it
// did not know, and the model understands more of them than this client does.
// It is also what a tool hands to whoever registered it, so leaving it as
// written keeps the registry free of a second copy of the schema.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // a JSON Schema object
}

// ToolCall is a call the model asked for.
//
// It is the wire shape rather than a parsed one. The arguments arrive as a JSON
// document that whoever runs the call parses, and a transport that parsed them
// would have to know what each tool takes, which is the tool's business rather
// than the endpoint's. It is the same shape the assistant message is replayed
// with, since the model has to be shown the calls in the form it wrote them.
type ToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction names the function a call asks for and the arguments for it.
//
// The name is what a call is run by, so it is required rather than inferred
// from the arguments, and a call carrying none is not a call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // a JSON document, unparsed
}
