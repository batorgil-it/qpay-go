package qpay_quick

import (
	"fmt"
	"time"
)

// registerDefaultCallbacks wires the core QPay Quick HTTP execution into each
// operation processor. These named entries form the baseline hook chain;
// plugins anchor their own hooks relative to these names, e.g.:
//
//	e.Callback().CreateInvoice().Before("qpay:create_invoice").Register(...)
//	e.Callback().CreateInvoice().After("qpay:create_invoice").Register(...)
func registerDefaultCallbacks(q *qpayquick) {
	mustRegister := func(p *Processor, name string, fn func(*Context)) {
		if err := p.Register(name, fn); err != nil {
			panic("qpay-quick: default callback registration failed: " + err.Error())
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
			SetBody(map[string]string{"terminal_id": ctx.client.terminalID}).
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

	mustRegister(q.cbs.HttpRequest(), "qpay:http_request", func(ctx *Context) {
		if ctx.Error != nil {
			return
		}
		ctx.AddError(ctx.client.httpRequestQPay(
			ctx.Statement.Context,
			ctx.Statement.Request,
			ctx.Statement.Response,
			ctx.Statement.API,
			ctx.Statement.URLExt,
		))
	})

	// ── API operations ────────────────────────────────────────────────────────

	executeHTTP := func(ctx *Context) {
		if ctx.Error != nil {
			return
		}
		q.cbs.HttpRequest().Execute(ctx)
	}

	mustRegister(q.cbs.CreateCompany(), "qpay:create_company", executeHTTP)
	mustRegister(q.cbs.CreatePerson(), "qpay:create_person", executeHTTP)
	mustRegister(q.cbs.UpdateCompany(), "qpay:update_company", executeHTTP)
	mustRegister(q.cbs.UpdatePerson(), "qpay:update_person", executeHTTP)
	mustRegister(q.cbs.GetMerchant(), "qpay:get_merchant", executeHTTP)
	mustRegister(q.cbs.DeleteMerchant(), "qpay:delete_merchant", executeHTTP)
	mustRegister(q.cbs.ListMerchant(), "qpay:list_merchant", executeHTTP)
	mustRegister(q.cbs.GetAimagHot(), "qpay:get_aimag_hot", executeHTTP)
	mustRegister(q.cbs.GetSumDuureg(), "qpay:get_sum_duureg", executeHTTP)
	mustRegister(q.cbs.CreateInvoice(), "qpay:create_invoice", executeHTTP)
	mustRegister(q.cbs.GetInvoice(), "qpay:get_invoice", executeHTTP)
	mustRegister(q.cbs.CancelInvoice(), "qpay:cancel_invoice", executeHTTP)
	mustRegister(q.cbs.CheckPayment(), "qpay:check_payment", executeHTTP)
}
