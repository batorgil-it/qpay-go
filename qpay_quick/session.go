package qpay_quick

import "context"

// withContextQuick is a thin wrapper around *qpayquick that injects a
// caller-supplied context.Context into every operation's Statement. It is
// returned by (*qpayquick).WithContext and satisfies the QPayQuick interface.
type withContextQuick struct {
	q   *qpayquick
	ctx context.Context
}

// WithContext returns a new session with the given context.
func (s *withContextQuick) WithContext(ctx context.Context) QPayQuick {
	if ctx == nil {
		ctx = context.Background()
	}
	return &withContextQuick{q: s.q, ctx: ctx}
}

// Use delegates to the underlying client.
func (s *withContextQuick) Use(plugin Plugin) error { return s.q.Use(plugin) }

// Callback delegates to the underlying client.
func (s *withContextQuick) Callback() *Callbacks { return s.q.Callback() }

func (s *withContextQuick) CreateCompany(input QpayCompanyCreateRequest) (QpayCompanyCreateResponse, error) {
	var response QpayCompanyCreateResponse
	ctx := s.q.newContext(s.ctx, "create_company", input, &response, QPayCreateCompany, "")
	s.q.cbs.CreateCompany().Execute(ctx)
	if ctx.Error != nil {
		return QpayCompanyCreateResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) CreatePerson(input QpayPersonCreateRequest) (QpayPersonCreateResponse, error) {
	var response QpayPersonCreateResponse
	ctx := s.q.newContext(s.ctx, "create_person", input, &response, QPayCreatePerson, "")
	s.q.cbs.CreatePerson().Execute(ctx)
	if ctx.Error != nil {
		return QpayPersonCreateResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) UpdateCompany(merchantID string, input QpayCompanyCreateRequest) (QpayCompanyCreateResponse, error) {
	var response QpayCompanyCreateResponse
	ctx := s.q.newContext(s.ctx, "update_company", input, &response, QPayUpdateCompany, merchantID)
	s.q.cbs.UpdateCompany().Execute(ctx)
	if ctx.Error != nil {
		return QpayCompanyCreateResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) UpdatePerson(merchantID string, input QpayPersonCreateRequest) (QpayPersonCreateResponse, error) {
	var response QpayPersonCreateResponse
	ctx := s.q.newContext(s.ctx, "update_person", input, &response, QPayUpdatePerson, merchantID)
	s.q.cbs.UpdatePerson().Execute(ctx)
	if ctx.Error != nil {
		return QpayPersonCreateResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) GetMerchant(merchantID string) (QpayMerchantGetResponse, error) {
	var response QpayMerchantGetResponse
	ctx := s.q.newContext(s.ctx, "get_merchant", nil, &response, QPayGetMerchant, merchantID)
	s.q.cbs.GetMerchant().Execute(ctx)
	if ctx.Error != nil {
		return QpayMerchantGetResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) DeleteMerchant(merchantID string) (QpayGeneralResponse, error) {
	var response QpayGeneralResponse
	ctx := s.q.newContext(s.ctx, "delete_merchant", nil, &response, QPayDeleteMerchant, merchantID)
	s.q.cbs.DeleteMerchant().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) ListMerchant(page, limit int64) (QpayMerchantListResponse, error) {
	request := QpayMerchantListRequest{Page: page, Limit: limit}
	var response QpayMerchantListResponse
	ctx := s.q.newContext(s.ctx, "list_merchant", request, &response, QPayMerchantList, "")
	s.q.cbs.ListMerchant().Execute(ctx)
	if ctx.Error != nil {
		return QpayMerchantListResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) GetAimagHot() ([]QpayLocationCode, error) {
	var response []QpayLocationCode
	ctx := s.q.newContext(s.ctx, "get_aimag_hot", nil, &response, QPayGetAimagHot, "")
	s.q.cbs.GetAimagHot().Execute(ctx)
	if ctx.Error != nil {
		return nil, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) GetSumDuureg(aimagHotCode string) ([]QpayLocationCode, error) {
	var response []QpayLocationCode
	ctx := s.q.newContext(s.ctx, "get_sum_duureg", nil, &response, QPayGetSumDuureg, aimagHotCode)
	s.q.cbs.GetSumDuureg().Execute(ctx)
	if ctx.Error != nil {
		return nil, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) CreateInvoice(input QpayInvoiceRequest) (QpayInvoiceResponse, error) {
	if input.CallbackUrl == "" {
		input.CallbackUrl = s.q.callback
	}
	var response QpayInvoiceResponse
	ctx := s.q.newContext(s.ctx, "create_invoice", input, &response, QPayInvoiceCreate, "")
	s.q.cbs.CreateInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayInvoiceResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) GetInvoice(invoiceId string) (QpayInvoiceGetResponse, error) {
	var response QpayInvoiceGetResponse
	ctx := s.q.newContext(s.ctx, "get_invoice", nil, &response, QPayInvoiceGet, invoiceId)
	s.q.cbs.GetInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayInvoiceGetResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) CancelInvoice(invoiceId string) (QpayGeneralResponse, error) {
	var response QpayGeneralResponse
	ctx := s.q.newContext(s.ctx, "cancel_invoice", nil, &response, QPayInvoiceCancel, invoiceId)
	s.q.cbs.CancelInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

func (s *withContextQuick) CheckPayment(invoiceID string) (QpayPaymentCheckResponse, error) {
	request := QpayPaymentCheckRequest{InvoiceID: invoiceID}
	var response QpayPaymentCheckResponse
	ctx := s.q.newContext(s.ctx, "check_payment", request, &response, QPayPaymentCheck, "")
	s.q.cbs.CheckPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayPaymentCheckResponse{}, ctx.Error
	}
	return response, nil
}
