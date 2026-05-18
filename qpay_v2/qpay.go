package qpay_v2

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/batorgil-it/qpay-go/utils"
	"golang.org/x/sync/singleflight"
	"resty.dev/v3"
)

type qpay struct {
	endpoint    string
	password    string
	username    string
	callback    string
	invoiceCode string
	merchantId  string
	syncAuth    bool // If true, New() blocks until auth completes
	loginObject *qpayLoginResponse
	mu          sync.RWMutex
	authGroup   singleflight.Group // Coalesces concurrent auth calls into one
	client      *resty.Client
	plugins     map[string]Plugin
	cbs         *Callbacks
}

// QPay [QPay V2 SDK Interface / Интерфэйс]
type QPay interface {
	// CreateInvoice [Төлбөрийн нэхэмжлэл үүсгэх]
	// See: https://developer.qpay.mn/#invoice-Create
	CreateInvoice(input QPayCreateInvoiceInput) (QPaySimpleInvoiceResponse, error)

	// CreateEbarimtInvoice [И-баримт 3.0 мэдээлэлтэй нэхэмжлэх үүсгэх]
	CreateEbarimtInvoice(input QPayCreateEbarimtInvoiceInput) (QPaySimpleInvoiceResponse, error)

	// GetInvoice [Үүсгэсэн нэхэмжлэлийн мэдээлэл харах]
	// See: https://developer.qpay.mn/#invoice-Get
	GetInvoice(invoiceId string) (QpayInvoiceGetResponse, error)

	// CancelInvoice [Нэхэмжлэх цуцлах]
	// See: https://developer.qpay.mn/#invoice-Cancel
	CancelInvoice(invoiceId string) (QpayGeneralResponse, error)

	// GetPayment [Төлбөрийн мэдээлэл татах]
	// See: https://developer.qpay.mn/#payment-Get
	GetPayment(paymentId string) (QpayTransaction, error)

	// CheckPayment [Төлбөр төлөгдсөн эсэхийг шалгах]
	// See: https://developer.qpay.mn/#payment-check
	CheckPayment(invoiceId string, pageLimit, pageNumber int64) (QpayPaymentCheckResponse, error)

	// CancelPayment [Төлөгдсөн төлбөрийг цуцлах]
	// See: https://developer.qpay.mn/#payment-cancel
	CancelPayment(invoiceId, paymentId string) (QpayGeneralResponse, error)

	// RefundPayment [Төлбөр буцаах]
	// See: https://developer.qpay.mn/#payment-refund
	RefundPayment(invoiceId, paymentId string) (QpayGeneralResponse, error)

	// GetPaymentList [Төлбөрийн жагсаалт авах]
	// See: https://developer.qpay.mn/#payment-list
	GetPaymentList(input QPayPaymentListInput) (QpayPaymentListResponse, error)

	// CreateEbarimt [Төлбөр төлөгдсөний дараа и-баримт 3.0 үүсгэх]
	CreateEbarimt(input QPayEbarimtCreateInput) (QPayEbarimtResponse, error)

	// CancelEbarimt [И-баримт 3.0 цуцлах]
	CancelEbarimt(paymentId string) (QPayEbarimtResponse, error)

	// WithContext returns a shallow session that injects ctx into every
	// subsequent operation's Statement, enabling span propagation for plugins
	// such as the OpenTelemetry tracing plugin.
	//
	// Usage:
	//   client.WithContext(ctx).CreateInvoice(input)
	WithContext(ctx context.Context) QPay

	// Use registers a plugin with the client.
	// Returns ErrPluginRegistered if a plugin with the same name already exists.
	// Initialize is called before the plugin is stored, so a failing Initialize
	// leaves the plugin map unchanged.
	Use(plugin Plugin) error

	// Callback returns the callback registry, giving direct access to every
	// operation's Processor for registering hooks outside of a full Plugin.
	Callback() *Callbacks
}

// Option defines an option for qpay initialization.
type Option func(*qpay)

// WithClient [Custom resty.Client ашиглах]
// This is useful for injecting a client with custom timeouts, certificates, etc.
func WithClient(client *resty.Client) Option {
	return func(q *qpay) {
		if client != nil {
			q.client = client
		}
	}
}

// WithSyncAuth [Эхлүүлэхдээ auth дуустал хүлээх]
// By default, auth runs in the background so New() returns immediately.
// Use this option to block until auth completes — useful when you need
// a valid token before making the first API call.
func WithSyncAuth() Option {
	return func(q *qpay) {
		q.syncAuth = true
	}
}

// New [QPay V2 SDK-ийг шинээр үүсгэх]
// username: qPay-ээс өгсөн хэрэглэгчийн нэр (client_id)
// password: qPay-ээс өгсөн нууц үг (client_secret)
// endpoint: Sandbox эсвэл Production хаяг
// callback: Төлбөр төлөгдсөний дараа дуудагдах URL
// invoiceCode: qPay нэхэмжлэхийн код
// merchantId: Байгууллагын ID
func New(username, password, endpoint, callback, invoiceCode, merchantId string, options ...Option) QPay {
	q := &qpay{
		endpoint:    endpoint,
		password:    password,
		username:    username,
		callback:    callback,
		invoiceCode: invoiceCode,
		merchantId:  merchantId,
		client:      resty.New().SetTransport(newTransport()).SetTimeout(60 * time.Second),
		plugins:     map[string]Plugin{},
	}

	for _, opt := range options {
		opt(q)
	}

	q.cbs = initializeCallbacks(q)
	registerDefaultCallbacks(q)

	if q.syncAuth {
		// Block until auth completes with retry.
		// If QPay is temporarily unreachable, retry up to 3 times
		// with a 1-second delay between attempts.
		for i := 0; i < 3; i++ {
			if _, err := q.authQPayV2(); err == nil {
				break
			}
			if i < 2 {
				time.Sleep(1 * time.Second)
			}
		}
	} else {
		// Attempt login in background to warm the token cache.
		// If it fails, authQPayV2 will retry on the first API call.
		go q.authQPayV2() //nolint:errcheck
	}

	return q
}

// WithContext returns a session that propagates ctx into every operation's
// Statement.Context, so plugins (e.g. the OTel tracing plugin) can start
// child spans under the caller's trace.
func (q *qpay) WithContext(ctx context.Context) QPay {
	if ctx == nil {
		ctx = context.Background()
	}
	return &withContextQPay{q: q, ctx: ctx}
}

// Use registers a plugin with the client. Initialize is called before the
// plugin is stored — a failing Initialize leaves the plugin map unchanged.
func (q *qpay) Use(plugin Plugin) error {
	name := plugin.Name()
	if _, ok := q.plugins[name]; ok {
		return ErrPluginRegistered
	}
	if err := plugin.Initialize(q); err != nil {
		return err
	}
	q.plugins[name] = plugin
	return nil
}

// Callback returns the callback registry for direct hook registration.
// It implements the Hooks interface so *qpay can be passed to Plugin.Initialize.
func (q *qpay) Callback() *Callbacks {
	return q.cbs
}

// newContext constructs a per-request Context ready for processor.Execute.
// goCtx is stored in Statement.Context so plugins can propagate traces.
func (q *qpay) newContext(goCtx context.Context, op string, req interface{}, resp interface{}, api utils.API, urlExt string) *Context {
	return &Context{
		client: q,
		Statement: &Statement{
			Context:   goCtx,
			Operation: op,
			Request:   req,
			Response:  resp,
			API:       api,
			URLExt:    urlExt,
		},
	}
}

// buildCreateInvoiceRequest assembles the wire request from user input.
// Extracted so that the WithContext session can reuse it without duplication.
func (q *qpay) buildCreateInvoiceRequest(input QPayCreateInvoiceInput) QPaySimpleInvoiceRequest {
	vals := url.Values{}
	for k, v := range input.CallbackParam {
		vals.Add(k, v)
	}
	callbackUrl := q.callback
	if len(vals) > 0 {
		callbackUrl = fmt.Sprintf("%s?%s", q.callback, vals.Encode())
	}

	var minAmt *int64
	if input.MinimumAmount > 0 {
		minAmt = &input.MinimumAmount
	}
	var maxAmt *int64
	if input.MaximumAmount > 0 {
		maxAmt = &input.MaximumAmount
	}

	return QPaySimpleInvoiceRequest{
		InvoiceCode:         q.invoiceCode,
		SenderInvoiceNo:     input.SenderInvoiceNo,
		SenderBranchCode:    input.SenderBranchCode,
		SenderBranchData:    input.SenderBranchData,
		SenderTerminalCode:  input.SenderTerminalCode,
		SenderTerminalData:  input.SenderTerminalData,
		SenderStaffCode:     input.SenderStaffCode,
		SenderStaffData:     input.SenderStaffData,
		InvoiceReceiverCode: input.InvoiceReceiverCode,
		InvoiceReceiverData: input.InvoiceReceiverData,
		InvoiceDescription:  input.InvoiceDescription,
		Amount:              input.Amount,
		CallbackUrl:         callbackUrl,
		InvoiceDueDate:      input.InvoiceDueDate,
		ExpiryDate:          input.ExpiryDate,
		EnableExpiry:        input.EnableExpiry,
		AllowPartial:        input.AllowPartial,
		MinimumAmount:       minAmt,
		AllowExceed:         input.AllowExceed,
		MaximumAmount:       maxAmt,
		CalculateVat:        input.CalculateVat,
		Lines:               input.Lines,
		Note:                input.Note,
		TaxCustomerCode:     input.TaxCustomerCode,
		LineTaxCode:         input.LineTaxCode,
		Transactions:        input.Transactions,
	}
}

// buildGetPaymentListRequest assembles the list request with defaults applied.
func (q *qpay) buildGetPaymentListRequest(input QPayPaymentListInput) QpayPaymentListRequest {
	objType := input.ObjectType
	if objType == "" {
		objType = "MERCHANT"
	}
	objID := input.ObjectID
	if objID == "" {
		objID = q.merchantId
	}
	return QpayPaymentListRequest{
		ObjectType:           objType,
		ObjectID:             objID,
		MerchantBranchCode:   input.BranchCode,
		MerchantTerminalCode: input.TerminalCode,
		MerchantStaffCode:    input.StaffCode,
		Offset: QpayOffset{
			PageLimit:  input.PageLimit,
			PageNumber: input.PageNumber,
		},
	}
}

// CreateInvoice [Нэхэмжлэх үүсгэх]
func (q *qpay) CreateInvoice(input QPayCreateInvoiceInput) (QPaySimpleInvoiceResponse, error) {
	request := q.buildCreateInvoiceRequest(input)
	var response QPaySimpleInvoiceResponse
	ctx := q.newContext(context.Background(), "create_invoice", request, &response, QPayInvoiceCreate, "")
	q.cbs.CreateInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QPaySimpleInvoiceResponse{}, ctx.Error
	}
	return response, nil
}

// CreateEbarimtInvoice [И-баримт 3.0 мэдээлэлтэй нэхэмжлэх үүсгэх]
func (q *qpay) CreateEbarimtInvoice(input QPayCreateEbarimtInvoiceInput) (QPaySimpleInvoiceResponse, error) {
	request := q.newEbarimtInvoiceRequest(input)
	var response QPaySimpleInvoiceResponse
	ctx := q.newContext(context.Background(), "create_ebarimt_invoice", request, &response, QPayInvoiceCreate, "")
	q.cbs.CreateEbarimtInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QPaySimpleInvoiceResponse{}, ctx.Error
	}
	return response, nil
}

func (q *qpay) newEbarimtInvoiceRequest(input QPayCreateEbarimtInvoiceInput) QPayEbarimtInvoiceRequest {
	vals := url.Values{}
	for k, v := range input.CallbackParam {
		vals.Add(k, v)
	}

	callbackURL := input.CallbackURL
	if callbackURL == "" {
		callbackURL = q.callback
	}
	if len(vals) > 0 {
		callbackURL = fmt.Sprintf("%s?%s", callbackURL, vals.Encode())
	}

	invoiceCode := input.InvoiceCode
	if invoiceCode == "" {
		invoiceCode = q.invoiceCode
	}

	calculateVat := input.CalculateVat
	if calculateVat == nil && (input.TaxType == QPayTaxTypeNoVAT || input.TaxType == QPayTaxTypeVATExempt) {
		value := false
		calculateVat = &value
	}

	return QPayEbarimtInvoiceRequest{
		InvoiceCode:         invoiceCode,
		SenderInvoiceNo:     input.SenderInvoiceNo,
		SenderBranchCode:    input.SenderBranchCode,
		SenderStaffCode:     input.SenderStaffCode,
		SenderStaffData:     input.SenderStaffData,
		InvoiceReceiverCode: input.InvoiceReceiverCode,
		InvoiceReceiverData: input.InvoiceReceiverData,
		InvoiceDescription:  input.InvoiceDescription,
		TaxType:             input.TaxType,
		DistrictCode:        input.DistrictCode,
		CallbackUrl:         callbackURL,
		CalculateVat:        calculateVat,
		Lines:               input.Lines,
	}
}

// GetInvoice [Нэхэмжлэхийн мэдээлэл авах]
func (q *qpay) GetInvoice(invoiceId string) (QpayInvoiceGetResponse, error) {
	var response QpayInvoiceGetResponse
	ctx := q.newContext(context.Background(), "get_invoice", nil, &response, QPayInvoiceGet, invoiceId)
	q.cbs.GetInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayInvoiceGetResponse{}, ctx.Error
	}
	return response, nil
}

// CancelInvoice [Үүсгэсэн нэхэмжлэлийг цуцлах]
func (q *qpay) CancelInvoice(invoiceId string) (QpayGeneralResponse, error) {
	var response QpayGeneralResponse
	ctx := q.newContext(context.Background(), "cancel_invoice", nil, &response, QPayInvoiceCancel, invoiceId)
	q.cbs.CancelInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

// GetPayment [Төлбөрийн мэдээлэл татах]
func (q *qpay) GetPayment(paymentId string) (QpayTransaction, error) {
	var response QpayTransaction
	ctx := q.newContext(context.Background(), "get_payment", nil, &response, QPayPaymentGet, paymentId)
	q.cbs.GetPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayTransaction{}, ctx.Error
	}
	return response, nil
}

// CheckPayment [Нэхэмжлэлийн төлбөрийг шалгах]
func (q *qpay) CheckPayment(invoiceId string, pageLimit, pageNumber int64) (QpayPaymentCheckResponse, error) {
	req := QpayPaymentCheckRequest{
		ObjectType: "INVOICE",
		ObjectID:   invoiceId,
		Offset: QpayOffset{
			PageLimit:  pageLimit,
			PageNumber: pageNumber,
		},
	}
	var response QpayPaymentCheckResponse
	ctx := q.newContext(context.Background(), "check_payment", req, &response, QPayPaymentCheck, "")
	q.cbs.CheckPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayPaymentCheckResponse{}, ctx.Error
	}
	return response, nil
}

// CancelPayment [Төлөгдсөн төлбөрийг цуцлах]
func (q *qpay) CancelPayment(invoiceId, paymentId string) (QpayGeneralResponse, error) {
	req := QpayPaymentCancelRequest{
		CallbackUrl: q.callback,
		Note:        "Cancel payment for invoice: " + invoiceId,
	}
	var response QpayGeneralResponse
	ctx := q.newContext(context.Background(), "cancel_payment", req, &response, QPayPaymentCancel, paymentId)
	q.cbs.CancelPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

// RefundPayment [Төлбөр буцаалт хийх]
func (q *qpay) RefundPayment(invoiceId, paymentId string) (QpayGeneralResponse, error) {
	req := QpayPaymentCancelRequest{
		CallbackUrl: q.callback,
		Note:        "Refund payment for invoice: " + invoiceId,
	}
	var response QpayGeneralResponse
	ctx := q.newContext(context.Background(), "refund_payment", req, &response, QPayPaymentRefund, paymentId)
	q.cbs.RefundPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

// newTransport creates an http.Transport with sensible defaults.
func newTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       20,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// GetPaymentList [Төлбөр төлөлтийн жагсаалт авах]
func (q *qpay) GetPaymentList(input QPayPaymentListInput) (QpayPaymentListResponse, error) {
	req := q.buildGetPaymentListRequest(input)
	var response QpayPaymentListResponse
	ctx := q.newContext(context.Background(), "get_payment_list", req, &response, QPayPaymentList, "")
	q.cbs.GetPaymentList().Execute(ctx)
	if ctx.Error != nil {
		return QpayPaymentListResponse{}, ctx.Error
	}
	return response, nil
}

// CreateEbarimt [Төлбөр төлөгдсөний дараа и-баримт 3.0 үүсгэх]
func (q *qpay) CreateEbarimt(input QPayEbarimtCreateInput) (QPayEbarimtResponse, error) {
	request := QPayEbarimtCreateRequest{
		PaymentID:           input.PaymentID,
		EbarimtReceiverType: input.EbarimtReceiverType,
		EbarimtReceiver:     input.EbarimtReceiver,
		DistrictCode:        input.DistrictCode,
		ClassificationCode:  input.ClassificationCode,
	}
	var response QPayEbarimtResponse
	ctx := q.newContext(context.Background(), "create_ebarimt", request, &response, QPayEbarimtCreate, "")
	q.cbs.CreateEbarimt().Execute(ctx)
	if ctx.Error != nil {
		return QPayEbarimtResponse{}, ctx.Error
	}
	return response, nil
}

// CancelEbarimt [И-баримт 3.0 цуцлах]
func (q *qpay) CancelEbarimt(paymentId string) (QPayEbarimtResponse, error) {
	var response QPayEbarimtResponse
	ctx := q.newContext(context.Background(), "cancel_ebarimt", nil, &response, QPayEbarimtCancel, paymentId)
	q.cbs.CancelEbarimt().Execute(ctx)
	if ctx.Error != nil {
		return QPayEbarimtResponse{}, ctx.Error
	}
	return response, nil
}
