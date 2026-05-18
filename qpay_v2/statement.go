package qpay_v2

import (
	"context"
	"sync"

	"github.com/batorgil-it/qpay-go/utils"
)

// Statement carries per-request data that flows through the callback chain.
// Plugins can use Settings to store and retrieve per-request state without
// colliding with other plugins — use a unique key such as "myplugin:key".
type Statement struct {
	Context   context.Context // request-scoped context; set by WithContext
	Settings  sync.Map        // plugin key/value storage (safe for concurrent use)
	Operation string          // operation name, e.g. "create_invoice"
	Request   interface{}     // outbound request body (nil for GET/DELETE)
	Response  interface{}     // pointer to response struct; populated after HTTP call
	API       utils.API       // endpoint configuration
	URLExt    string          // URL path suffix appended to API.Url
}

// Context is the execution context passed to every callback function.
// Default callbacks (internal to this package) use the unexported client
// field to make the underlying HTTP call. Plugin callbacks interact with
// the context through Statement and Error only.
type Context struct {
	Statement *Statement
	Error     error
	client    *qpay
}

// AddError records the first non-nil error encountered during processing.
// Subsequent calls are no-ops if an error is already set.
func (c *Context) AddError(err error) {
	if err != nil && c.Error == nil {
		c.Error = err
	}
}
