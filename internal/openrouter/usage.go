package openrouter

// usageBody is the shape that asks the endpoint to report token accounting.
//
// The field is placed on the request body rather than in a header, since the
// endpoint reads it from there. The accounting is reported alongside the
// generation and does not change what is generated, but it is asked for
// explicitly rather than assumed, since an endpoint that is not asked for it
// sends nothing and the counters would otherwise be permanently blank.
type usageBody struct {
	ChatRequest
	// Stream is set here rather than carried by the request, since a reply is
	// always streamed. It is sent because the endpoint expects it, not
	// because a caller may turn it off.
	Stream bool `json:"stream"`
	// Usage is an object rather than a boolean because the endpoint expects
	// the object form, and an object carrying include is the narrowest thing
	// that can be asked for.
	Usage struct {
		Include bool `json:"include"`
	} `json:"usage"`
}

// withUsage returns the request with token accounting requested.
func withUsage(req ChatRequest) usageBody {
	var body usageBody
	body.ChatRequest = req
	body.Stream = true
	body.Usage.Include = true
	return body
}
