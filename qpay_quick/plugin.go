package qpay_quick

import (
	"errors"
	"fmt"
	"sort"
)

// Plugin is the extension contract every qpay-quick plugin must satisfy.
// Name must be globally unique — it is used as the registry key.
// Initialize is called exactly once when the plugin is registered via Use().
type Plugin interface {
	Name() string
	Initialize(Hooks) error
}

// Hooks is the interface passed to Plugin.Initialize.
// It provides write access to the client's callback pipeline so plugins can
// register, remove, or replace named hooks without touching internal state.
type Hooks interface {
	Callback() *Callbacks
}

// ErrPluginRegistered is returned by Use when a plugin with the same name
// has already been registered.
var ErrPluginRegistered = errors.New("plugin already registered")

// ── Callback registry ─────────────────────────────────────────────────────────

// Callbacks holds one Processor per QPay Quick operation type.
type Callbacks struct {
	processors map[string]*Processor
}

func initializeCallbacks(q *qpayquick) *Callbacks {
	return &Callbacks{
		processors: map[string]*Processor{
			"create_company":  {client: q},
			"create_person":   {client: q},
			"update_company":  {client: q},
			"update_person":   {client: q},
			"get_merchant":    {client: q},
			"delete_merchant": {client: q},
			"list_merchant":   {client: q},
			"get_aimag_hot":   {client: q},
			"get_sum_duureg":  {client: q},
			"create_invoice":  {client: q},
			"get_invoice":     {client: q},
			"cancel_invoice":  {client: q},
			"check_payment":   {client: q},
		},
	}
}

// Operation-specific accessor methods — plugins use these to target hooks.
func (c *Callbacks) CreateCompany() *Processor  { return c.processors["create_company"] }
func (c *Callbacks) CreatePerson() *Processor   { return c.processors["create_person"] }
func (c *Callbacks) UpdateCompany() *Processor  { return c.processors["update_company"] }
func (c *Callbacks) UpdatePerson() *Processor   { return c.processors["update_person"] }
func (c *Callbacks) GetMerchant() *Processor    { return c.processors["get_merchant"] }
func (c *Callbacks) DeleteMerchant() *Processor { return c.processors["delete_merchant"] }
func (c *Callbacks) ListMerchant() *Processor   { return c.processors["list_merchant"] }
func (c *Callbacks) GetAimagHot() *Processor    { return c.processors["get_aimag_hot"] }
func (c *Callbacks) GetSumDuureg() *Processor   { return c.processors["get_sum_duureg"] }
func (c *Callbacks) CreateInvoice() *Processor  { return c.processors["create_invoice"] }
func (c *Callbacks) GetInvoice() *Processor     { return c.processors["get_invoice"] }
func (c *Callbacks) CancelInvoice() *Processor  { return c.processors["cancel_invoice"] }
func (c *Callbacks) CheckPayment() *Processor   { return c.processors["check_payment"] }

// ── Processor ─────────────────────────────────────────────────────────────────

// Processor manages the ordered list of hooks for one operation type.
// The compiled fns slice is rebuilt each time Register/Remove/Replace is
// called (at init time only), so runtime cost is a plain slice iteration.
type Processor struct {
	client    *qpayquick
	fns       []func(*Context) // compiled, sorted — used at runtime
	callbacks []*callback      // raw registrations — mutated only at init
}

// Execute runs the compiled hook chain on ctx, stopping at the first error.
func (p *Processor) Execute(ctx *Context) *Context {
	for _, fn := range p.fns {
		fn(ctx)
		if ctx.Error != nil {
			break
		}
	}
	return ctx
}

// Register appends a named hook to this processor's chain.
func (p *Processor) Register(name string, fn func(*Context)) error {
	return (&callback{processor: p}).Register(name, fn)
}

// Before returns a fluent builder that places the registered hook
// immediately before the hook named name in the execution order.
func (p *Processor) Before(name string) *callback {
	return &callback{before: name, processor: p}
}

// After returns a fluent builder that places the registered hook
// immediately after the hook named name in the execution order.
func (p *Processor) After(name string) *callback {
	return &callback{after: name, processor: p}
}

// Match returns a fluent builder with a compile-time inclusion predicate.
// fn is evaluated once when Register is called; if it returns false the hook
// is excluded from the compiled chain. Use the handler body for per-request
// conditions instead.
func (p *Processor) Match(fn func(*Context) bool) *callback {
	return &callback{match: fn, processor: p}
}

// Remove marks a named hook for removal from the compiled chain.
// Removing a non-existent name is a silent no-op.
func (p *Processor) Remove(name string) error {
	p.callbacks = append(p.callbacks, &callback{name: name, remove: true, processor: p})
	return p.compile()
}

// Replace swaps the handler for an existing named hook, preserving its
// position in the chain. The last Replace for a given name wins.
func (p *Processor) Replace(name string, fn func(*Context)) error {
	p.callbacks = append(p.callbacks, &callback{name: name, handler: fn, replace: true, processor: p})
	return p.compile()
}

// ── callback ──────────────────────────────────────────────────────────────────

// callback is a single named hook entry with optional ordering and match
// constraints. It is intentionally unexported; external code reaches it only
// through the builder chain returned by Processor.Before / After / Match.
type callback struct {
	name      string
	before    string
	after     string
	remove    bool
	replace   bool
	match     func(*Context) bool
	handler   func(*Context)
	processor *Processor
}

func (c *callback) Before(name string) *callback { c.before = name; return c }
func (c *callback) After(name string) *callback  { c.after = name; return c }

func (c *callback) Register(name string, fn func(*Context)) error {
	c.name = name
	c.handler = fn
	c.processor.callbacks = append(c.processor.callbacks, c)
	return c.processor.compile()
}

// ── compile + sort ────────────────────────────────────────────────────────────

// compile rebuilds the sorted fns slice from the raw callbacks list.
// Called after every Register / Remove / Replace — never at request time.
func (p *Processor) compile() error {
	var active []*callback
	removed := map[string]bool{}
	matchCtx := &Context{client: p.client}
	for _, cb := range p.callbacks {
		if cb.remove {
			removed[cb.name] = true
		}
		if cb.match == nil || cb.match(matchCtx) {
			active = append(active, cb)
		}
	}
	fns, err := sortCallbacks(active, removed)
	if err != nil {
		return err
	}
	p.fns = fns
	return nil
}

// sortCallbacks performs a topological sort driven by before/after constraints
// and returns the ordered slice of handler functions, omitting removed names.
func sortCallbacks(cs []*callback, removed map[string]bool) ([]func(*Context), error) {
	names := make([]string, 0, len(cs))
	for _, c := range cs {
		names = append(names, c.name)
	}

	var sorted []string

	// Before("*") items go first, After("*") items go last.
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[j].before == "*" && cs[i].before != "*" {
			return true
		}
		if cs[j].after == "*" && cs[i].after != "*" {
			return true
		}
		return false
	})

	var sortOne func(c *callback) error
	sortOne = func(c *callback) error {
		getRIdx := func(strs []string, s string) int {
			for i := len(strs) - 1; i >= 0; i-- {
				if strs[i] == s {
					return i
				}
			}
			return -1
		}

		if c.before != "" {
			if c.before == "*" {
				if getRIdx(sorted, c.name) == -1 {
					sorted = append([]string{c.name}, sorted...)
				}
			} else if idx := getRIdx(sorted, c.before); idx != -1 {
				if cur := getRIdx(sorted, c.name); cur == -1 {
					sorted = append(sorted[:idx], append([]string{c.name}, sorted[idx:]...)...)
				} else if cur > idx {
					return fmt.Errorf("conflicting callback %s before %s", c.name, c.before)
				}
			} else if idx := getRIdx(names, c.before); idx != -1 {
				cs[idx].after = c.name
			}
		}

		if c.after != "" {
			if c.after == "*" {
				if getRIdx(sorted, c.name) == -1 {
					sorted = append(sorted, c.name)
				}
			} else if getRIdx(sorted, c.after) != -1 {
				if getRIdx(sorted, c.name) == -1 {
					sorted = append(sorted, c.name)
				}
			} else if idx := getRIdx(names, c.after); idx != -1 {
				after := cs[idx]
				if after.before == "" {
					after.before = c.name
				}
				if err := sortOne(after); err != nil {
					return err
				}
				if err := sortOne(c); err != nil {
					return err
				}
			}
		}

		if getRIdx(sorted, c.name) == -1 {
			sorted = append(sorted, c.name)
		}
		return nil
	}

	for _, c := range cs {
		if err := sortOne(c); err != nil {
			return nil, err
		}
	}

	var fns []func(*Context)
	for _, name := range sorted {
		if removed[name] {
			continue
		}
		for i := len(cs) - 1; i >= 0; i-- {
			if cs[i].name == name && !cs[i].remove {
				fns = append(fns, cs[i].handler)
				break
			}
		}
	}
	return fns, nil
}
