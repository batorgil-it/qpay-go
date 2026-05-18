package qpay_v2

// registerDefaultCallbacks wires the core QPay HTTP execution into each
// operation processor. These named entries form the baseline hook chain;
// plugins anchor their own hooks relative to these names, e.g.:
//
//	e.Callback().CreateInvoice().Before("qpay:create_invoice").Register(...)
//	e.Callback().CreateInvoice().After("qpay:create_invoice").Register(...)
func registerDefaultCallbacks(q *qpay) {
	// executeHTTP is the core handler: it forwards Statement fields to the
	// underlying HTTP transport. All operation processors share it because
	// every QPay call goes through the same httpRequestQPay path.
	executeHTTP := func(ctx *Context) {
		if ctx.Error != nil {
			return
		}
		ctx.AddError(ctx.client.httpRequestQPay(
			ctx.Statement.Request,
			ctx.Statement.Response,
			ctx.Statement.API,
			ctx.Statement.URLExt,
		))
	}

	mustRegister := func(p *Processor, name string, fn func(*Context)) {
		if err := p.Register(name, fn); err != nil {
			panic("qpay: default callback registration failed: " + err.Error())
		}
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
