package qpay_v2

import (
	"fmt"
	"time"
)

// registerDefaultCallbacks wires the core QPay HTTP execution into each
// operation processor. These named entries form the baseline hook chain;
// plugins anchor their own hooks relative to these names, e.g.:
//
//	e.Callback().CreateInvoice().Before("qpay:create_invoice").Register(...)
//	e.Callback().CreateInvoice().After("qpay:create_invoice").Register(...)
func registerDefaultCallbacks(q *qpay) {
	mustRegister := func(p *Processor, name string, fn func(*Context)) {
		if err := p.Register(name, fn); err != nil {
			panic("qpay: default callback registration failed: " + err.Error())
		}
	}

	// ── Auth ──────────────────────────────────────────────────────────────────

	mustRegister(q.cbs.Auth(), "qpay:auth", func(ctx *Context) {
		if ctx.Error != nil {
			return
		}
		res, err := ctx.client.client.R().
			SetHeader("Content-Type", "application/json").
			SetBasicAuth(ctx.client.username, ctx.client.password).
			SetResult(ctx.Statement.Response).
			Post(ctx.client.endpoint + QPayAuthToken.Url)
		if err != nil {
			ctx.AddError(err)
			return
		}
		if res.IsError() {
			ctx.AddError(fmt.Errorf("%s-QPay auth failed: %s (Status: %d)",
				time.Now().Format("2006-01-02 15:04:05"), res.String(), res.StatusCode()))
		}
	})

	mustRegister(q.cbs.RefreshAuth(), "qpay:refresh_auth", func(ctx *Context) {
		if ctx.Error != nil {
			return
		}
		refreshToken, _ := ctx.Statement.Request.(string)
		res, err := ctx.client.client.R().
			SetHeader("Content-Type", "application/json").
			SetAuthToken(refreshToken).
			SetResult(ctx.Statement.Response).
			Post(ctx.client.endpoint + QPayAuthRefresh.Url)
		if err != nil {
			ctx.AddError(err)
			return
		}
		if res.IsError() {
			ctx.AddError(fmt.Errorf("%s-QPay refresh failed: %s (Status: %d)",
				time.Now().Format("2006-01-02 15:04:05"), res.String(), res.StatusCode()))
		}
	})

	// ── HTTP transport ────────────────────────────────────────────────────────

	// qpay:http_request is the single hook point around every outbound QPay
	// HTTP call. It fires for all operations, sharing the operation's Context,
	// so plugins can use Before/After to wrap every request in one place.
	mustRegister(q.cbs.HttpRequest(), "qpay:http_request", func(ctx *Context) {
		if ctx.Error != nil {
			return
		}
		ctx.AddError(ctx.client.httpRequestQPay(
			ctx.Statement.Request,
			ctx.Statement.Response,
			ctx.Statement.API,
			ctx.Statement.URLExt,
		))
	})

	// ── API operations ────────────────────────────────────────────────────────

	// executeHTTP routes each operation through the shared http_request
	// processor, reusing the operation's Context so hooks on http_request
	// can access operation-level state (operation name, span, etc.).
	executeHTTP := func(ctx *Context) {
		if ctx.Error != nil {
			return
		}
		q.cbs.HttpRequest().Execute(ctx)
	}

	mustRegister(q.cbs.CreateInvoice(), "qpay:create_invoice", executeHTTP)
	mustRegister(q.cbs.CreateEbarimtInvoice(), "qpay:create_ebarimt_invoice", executeHTTP)
	mustRegister(q.cbs.GetInvoice(), "qpay:get_invoice", executeHTTP)
	mustRegister(q.cbs.CancelInvoice(), "qpay:cancel_invoice", executeHTTP)
	mustRegister(q.cbs.GetPayment(), "qpay:get_payment", executeHTTP)
	mustRegister(q.cbs.CheckPayment(), "qpay:check_payment", executeHTTP)
	mustRegister(q.cbs.CancelPayment(), "qpay:cancel_payment", executeHTTP)
	mustRegister(q.cbs.RefundPayment(), "qpay:refund_payment", executeHTTP)
	mustRegister(q.cbs.GetPaymentList(), "qpay:get_payment_list", executeHTTP)
	mustRegister(q.cbs.CreateEbarimt(), "qpay:create_ebarimt", executeHTTP)
	mustRegister(q.cbs.CancelEbarimt(), "qpay:cancel_ebarimt", executeHTTP)
}
