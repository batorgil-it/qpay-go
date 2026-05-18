package qpay_v2

import "context"

// withContextQPay is a thin wrapper around *qpay that injects a caller-supplied
// context.Context into every operation's Statement. It is returned by
// (*qpay).WithContext and satisfies the QPay interface.
type withContextQPay struct {
	q   *qpay
	ctx context.Context
}

// WithContext returns a new session with the given context, allowing chains like:
//
//	client.WithContext(ctx).WithContext(otherCtx).CreateInvoice(input)
func (s *withContextQPay) WithContext(ctx context.Context) QPay {
	if ctx == nil {
		ctx = context.Background()
	}
	return &withContextQPay{q: s.q, ctx: ctx}
}

// Use delegates to the underlying client.
func (s *withContextQPay) Use(plugin Plugin) error { return s.q.Use(plugin) }

// Callback delegates to the underlying client.
func (s *withContextQPay) Callback() *Callbacks { return s.q.Callback() }

func (s *withContextQPay) CreateInvoice(input QPayCreateInvoiceInput) (QPaySimpleInvoiceResponse, error) {
	request := s.q.buildCreateInvoiceRequest(input)
	var response QPaySimpleInvoiceResponse
	ctx := s.q.newContext(s.ctx, "create_invoice", request, &response, QPayInvoiceCreate, "")
	s.q.cbs.CreateInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QPaySimpleInvoiceResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) CreateEbarimtInvoice(input QPayCreateEbarimtInvoiceInput) (QPaySimpleInvoiceResponse, error) {
	request := s.q.newEbarimtInvoiceRequest(input)
	var response QPaySimpleInvoiceResponse
	ctx := s.q.newContext(s.ctx, "create_ebarimt_invoice", request, &response, QPayInvoiceCreate, "")
	s.q.cbs.CreateEbarimtInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QPaySimpleInvoiceResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) GetInvoice(invoiceId string) (QpayInvoiceGetResponse, error) {
	var response QpayInvoiceGetResponse
	ctx := s.q.newContext(s.ctx, "get_invoice", nil, &response, QPayInvoiceGet, invoiceId)
	s.q.cbs.GetInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayInvoiceGetResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) CancelInvoice(invoiceId string) (QpayGeneralResponse, error) {
	var response QpayGeneralResponse
	ctx := s.q.newContext(s.ctx, "cancel_invoice", nil, &response, QPayInvoiceCancel, invoiceId)
	s.q.cbs.CancelInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) GetPayment(paymentId string) (QpayTransaction, error) {
	var response QpayTransaction
	ctx := s.q.newContext(s.ctx, "get_payment", nil, &response, QPayPaymentGet, paymentId)
	s.q.cbs.GetPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayTransaction{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) CheckPayment(invoiceId string, pageLimit, pageNumber int64) (QpayPaymentCheckResponse, error) {
	req := QpayPaymentCheckRequest{
		ObjectType: "INVOICE",
		ObjectID:   invoiceId,
		Offset: QpayOffset{
			PageLimit:  pageLimit,
			PageNumber: pageNumber,
		},
	}
	var response QpayPaymentCheckResponse
	ctx := s.q.newContext(s.ctx, "check_payment", req, &response, QPayPaymentCheck, "")
	s.q.cbs.CheckPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayPaymentCheckResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) CancelPayment(invoiceId, paymentId string) (QpayGeneralResponse, error) {
	req := QpayPaymentCancelRequest{
		CallbackUrl: s.q.callback,
		Note:        "Cancel payment for invoice: " + invoiceId,
	}
	var response QpayGeneralResponse
	ctx := s.q.newContext(s.ctx, "cancel_payment", req, &response, QPayPaymentCancel, paymentId)
	s.q.cbs.CancelPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) RefundPayment(invoiceId, paymentId string) (QpayGeneralResponse, error) {
	req := QpayPaymentCancelRequest{
		CallbackUrl: s.q.callback,
		Note:        "Refund payment for invoice: " + invoiceId,
	}
	var response QpayGeneralResponse
	ctx := s.q.newContext(s.ctx, "refund_payment", req, &response, QPayPaymentRefund, paymentId)
	s.q.cbs.RefundPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) GetPaymentList(input QPayPaymentListInput) (QpayPaymentListResponse, error) {
	req := s.q.buildGetPaymentListRequest(input)
	var response QpayPaymentListResponse
	ctx := s.q.newContext(s.ctx, "get_payment_list", req, &response, QPayPaymentList, "")
	s.q.cbs.GetPaymentList().Execute(ctx)
	if ctx.Error != nil {
		return QpayPaymentListResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) CreateEbarimt(input QPayEbarimtCreateInput) (QPayEbarimtResponse, error) {
	request := QPayEbarimtCreateRequest{
		PaymentID:           input.PaymentID,
		EbarimtReceiverType: input.EbarimtReceiverType,
		EbarimtReceiver:     input.EbarimtReceiver,
		DistrictCode:        input.DistrictCode,
		ClassificationCode:  input.ClassificationCode,
	}
	var response QPayEbarimtResponse
	ctx := s.q.newContext(s.ctx, "create_ebarimt", request, &response, QPayEbarimtCreate, "")
	s.q.cbs.CreateEbarimt().Execute(ctx)
	if ctx.Error != nil {
		return QPayEbarimtResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQPay) CancelEbarimt(paymentId string) (QPayEbarimtResponse, error) {
	var response QPayEbarimtResponse
	ctx := s.q.newContext(s.ctx, "cancel_ebarimt", nil, &response, QPayEbarimtCancel, paymentId)
	s.q.cbs.CancelEbarimt().Execute(ctx)
	if ctx.Error != nil {
		return QPayEbarimtResponse{}, ctx.Error
	}
	return response, nil
}
