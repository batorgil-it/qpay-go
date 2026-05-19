package qpay_quick

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/batorgil-it/qpay-go/utils"
	"golang.org/x/sync/singleflight"
	"resty.dev/v3"
)

type qpayquick struct {
	endpoint    string
	password    string
	username    string
	callback    string
	terminalID  string
	syncAuth    bool // If true, New() blocks until auth completes
	loginObject *qpayLoginResponse
	mu          sync.RWMutex
	authGroup   singleflight.Group // Coalesces concurrent auth calls into one
	client      *resty.Client
	plugins     map[string]Plugin
	cbs         *Callbacks
}

// QPayQuick [QPay Quick Pay SDK Interface / Интерфэйс]
type QPayQuick interface {
	// CreateCompany [Байгууллага бүртгэх]
	CreateCompany(input QpayCompanyCreateRequest) (QpayCompanyCreateResponse, error)

	// CreatePerson [Хувь хүн бүртгэх]
	CreatePerson(input QpayPersonCreateRequest) (QpayPersonCreateResponse, error)

	// UpdateCompany [Байгууллагаар бүртгэсэн мерчантын мэдээлэл шинэчлэх]
	UpdateCompany(merchantID string, input QpayCompanyCreateRequest) (QpayCompanyCreateResponse, error)

	// UpdatePerson [Хувь хүнээр бүртгэсэн мерчантын мэдээлэл шинэчлэх]
	UpdatePerson(merchantID string, input QpayPersonCreateRequest) (QpayPersonCreateResponse, error)

	// GetMerchant [Мерчантын мэдээлэл харах]
	GetMerchant(merchantID string) (QpayMerchantGetResponse, error)

	// DeleteMerchant [Бүртгэлтэй мерчантыг устгах]
	DeleteMerchant(merchantID string) (QpayGeneralResponse, error)

	// ListMerchant [Мерчантуудын жагсаалт авах]
	ListMerchant(page, limit int64) (QpayMerchantListResponse, error)

	// GetAimagHot [Аймаг/хотын кодны жагсаалт]
	GetAimagHot() ([]QpayLocationCode, error)

	// GetSumDuureg [Сум/дүүргийн кодны жагсаалт (аймаг/хотын кодоор)]
	GetSumDuureg(aimagHotCode string) ([]QpayLocationCode, error)

	// CreateInvoice [Төлбөрийн нэхэмжлэл үүсгэх]
	CreateInvoice(input QpayInvoiceRequest) (QpayInvoiceResponse, error)

	// GetInvoice [Үүсгэсэн нэхэмжлэлийн мэдээлэл харах]
	GetInvoice(invoiceId string) (QpayInvoiceGetResponse, error)

	// CancelInvoice [Үүсгэсэн нэхэмжлэлийг цуцлах]
	CancelInvoice(invoiceId string) (QpayGeneralResponse, error)

	// CheckPayment [Төлбөр төлөгдсөн эсэхийг шалгах]
	CheckPayment(invoiceID string) (QpayPaymentCheckResponse, error)

	// WithContext returns a shallow session that injects ctx into every
	// subsequent operation's Statement, enabling span propagation for plugins
	// such as the OpenTelemetry tracing plugin.
	//
	// Usage:
	//   client.WithContext(ctx).CreateInvoice(input)
	WithContext(ctx context.Context) QPayQuick

	// Use registers a plugin with the client.
	// Returns ErrPluginRegistered if a plugin with the same name already exists.
	// Initialize is called before the plugin is stored, so a failing Initialize
	// leaves the plugin map unchanged.
	Use(plugin Plugin) error

	// Callback returns the callback registry, giving direct access to every
	// operation's Processor for registering hooks outside of a full Plugin.
	Callback() *Callbacks
}

// Option defines an option for qpayquick initialization.
type Option func(*qpayquick)

// WithClient [Custom resty.Client ашиглах]
// This is useful for injecting a client with custom timeouts, certificates, etc.
func WithClient(client *resty.Client) Option {
	return func(q *qpayquick) {
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
	return func(q *qpayquick) {
		q.syncAuth = true
	}
}

// New [QPay Quick SDK-ийг шинээр үүсгэх]
// username: qPay-ээс өгсөн хэрэглэгчийн нэр
// password: qPay-ээс өгсөн нууц үг
// endpoint: Sandbox эсвэл Production хаяг
// callback: Төлбөр төлөгдсөний дараа дуудагдах URL
// terminalID: qPay-ээс өгсөн терминалын дугаар
func New(username, password, endpoint, callback, terminalID string, options ...Option) QPayQuick {
	q := &qpayquick{
		endpoint:   endpoint,
		password:   password,
		username:   username,
		callback:   callback,
		terminalID: terminalID,
		client:     resty.New().SetTransport(newTransport()).SetTimeout(60 * time.Second),
		plugins:    map[string]Plugin{},
	}

	for _, opt := range options {
		opt(q)
	}

	q.cbs = initializeCallbacks(q)
	registerDefaultCallbacks(q)

	if q.syncAuth {
		for i := 0; i < 3; i++ {
			if _, err := q.authQPayV2(context.Background()); err == nil {
				break
			}
			if i < 2 {
				time.Sleep(1 * time.Second)
			}
		}
	} else {
		go q.authQPayV2(context.Background()) //nolint:errcheck
	}

	return q
}

// WithContext returns a session that propagates ctx into every operation's
// Statement.Context, so plugins (e.g. the OTel tracing plugin) can start
// child spans under the caller's trace.
func (q *qpayquick) WithContext(ctx context.Context) QPayQuick {
	if ctx == nil {
		ctx = context.Background()
	}
	return &withContextQuick{q: q, ctx: ctx}
}

// Use registers a plugin with the client. Initialize is called before the
// plugin is stored — a failing Initialize leaves the plugin map unchanged.
func (q *qpayquick) Use(plugin Plugin) error {
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
// It implements the Hooks interface so *qpayquick can be passed to Plugin.Initialize.
func (q *qpayquick) Callback() *Callbacks {
	return q.cbs
}

// newContext constructs a per-request Context ready for processor.Execute.
// goCtx is stored in Statement.Context so plugins can propagate traces.
func (q *qpayquick) newContext(goCtx context.Context, op string, req interface{}, resp interface{}, api utils.API, urlExt string) *Context {
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

// CreateCompany [Байгууллага бүртгэх]
func (q *qpayquick) CreateCompany(input QpayCompanyCreateRequest) (QpayCompanyCreateResponse, error) {
	var response QpayCompanyCreateResponse
	ctx := q.newContext(context.Background(), "create_company", input, &response, QPayCreateCompany, "")
	q.cbs.CreateCompany().Execute(ctx)
	if ctx.Error != nil {
		return QpayCompanyCreateResponse{}, ctx.Error
	}
	return response, nil
}

// CreatePerson [Хувь хүн бүртгэх]
func (q *qpayquick) CreatePerson(input QpayPersonCreateRequest) (QpayPersonCreateResponse, error) {
	var response QpayPersonCreateResponse
	ctx := q.newContext(context.Background(), "create_person", input, &response, QPayCreatePerson, "")
	q.cbs.CreatePerson().Execute(ctx)
	if ctx.Error != nil {
		return QpayPersonCreateResponse{}, ctx.Error
	}
	return response, nil
}

// GetMerchant [Мерчантын мэдээлэл харах]
func (q *qpayquick) GetMerchant(merchantID string) (QpayMerchantGetResponse, error) {
	var response QpayMerchantGetResponse
	ctx := q.newContext(context.Background(), "get_merchant", nil, &response, QPayGetMerchant, merchantID)
	q.cbs.GetMerchant().Execute(ctx)
	if ctx.Error != nil {
		return QpayMerchantGetResponse{}, ctx.Error
	}
	return response, nil
}

// UpdateCompany [Байгууллагаар бүртгэсэн мерчантын мэдээлэл шинэчлэх]
func (q *qpayquick) UpdateCompany(merchantID string, input QpayCompanyCreateRequest) (QpayCompanyCreateResponse, error) {
	var response QpayCompanyCreateResponse
	ctx := q.newContext(context.Background(), "update_company", input, &response, QPayUpdateCompany, merchantID)
	q.cbs.UpdateCompany().Execute(ctx)
	if ctx.Error != nil {
		return QpayCompanyCreateResponse{}, ctx.Error
	}
	return response, nil
}

// UpdatePerson [Хувь хүнээр бүртгэсэн мерчантын мэдээлэл шинэчлэх]
func (q *qpayquick) UpdatePerson(merchantID string, input QpayPersonCreateRequest) (QpayPersonCreateResponse, error) {
	var response QpayPersonCreateResponse
	ctx := q.newContext(context.Background(), "update_person", input, &response, QPayUpdatePerson, merchantID)
	q.cbs.UpdatePerson().Execute(ctx)
	if ctx.Error != nil {
		return QpayPersonCreateResponse{}, ctx.Error
	}
	return response, nil
}

// DeleteMerchant [Бүртгэлтэй мерчантыг устгах]
func (q *qpayquick) DeleteMerchant(merchantID string) (QpayGeneralResponse, error) {
	var response QpayGeneralResponse
	ctx := q.newContext(context.Background(), "delete_merchant", nil, &response, QPayDeleteMerchant, merchantID)
	q.cbs.DeleteMerchant().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

// ListMerchant [Мерчантуудын жагсаалт авах]
func (q *qpayquick) ListMerchant(page, limit int64) (QpayMerchantListResponse, error) {
	request := QpayMerchantListRequest{Page: page, Limit: limit}
	var response QpayMerchantListResponse
	ctx := q.newContext(context.Background(), "list_merchant", request, &response, QPayMerchantList, "")
	q.cbs.ListMerchant().Execute(ctx)
	if ctx.Error != nil {
		return QpayMerchantListResponse{}, ctx.Error
	}
	return response, nil
}

// GetAimagHot [Аймаг/хотын кодны жагсаалт]
func (q *qpayquick) GetAimagHot() ([]QpayLocationCode, error) {
	var response []QpayLocationCode
	ctx := q.newContext(context.Background(), "get_aimag_hot", nil, &response, QPayGetAimagHot, "")
	q.cbs.GetAimagHot().Execute(ctx)
	if ctx.Error != nil {
		return nil, ctx.Error
	}
	return response, nil
}

// GetSumDuureg [Сум/дүүргийн кодны жагсаалт]
func (q *qpayquick) GetSumDuureg(aimagHotCode string) ([]QpayLocationCode, error) {
	var response []QpayLocationCode
	ctx := q.newContext(context.Background(), "get_sum_duureg", nil, &response, QPayGetSumDuureg, aimagHotCode)
	q.cbs.GetSumDuureg().Execute(ctx)
	if ctx.Error != nil {
		return nil, ctx.Error
	}
	return response, nil
}

// CancelInvoice [Үүсгэсэн нэхэмжлэлийг цуцлах]
func (q *qpayquick) CancelInvoice(invoiceId string) (QpayGeneralResponse, error) {
	var response QpayGeneralResponse
	ctx := q.newContext(context.Background(), "cancel_invoice", nil, &response, QPayInvoiceCancel, invoiceId)
	q.cbs.CancelInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayGeneralResponse{}, ctx.Error
	}
	return response, nil
}

// CreateInvoice [Төлбөрийн нэхэмжлэл үүсгэх]
func (q *qpayquick) CreateInvoice(input QpayInvoiceRequest) (QpayInvoiceResponse, error) {
	if input.CallbackUrl == "" {
		input.CallbackUrl = q.callback
	}
	var response QpayInvoiceResponse
	ctx := q.newContext(context.Background(), "create_invoice", input, &response, QPayInvoiceCreate, "")
	q.cbs.CreateInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayInvoiceResponse{}, ctx.Error
	}
	return response, nil
}

// GetInvoice [Үүсгэсэн нэхэмжлэлийн мэдээлэл харах]
func (q *qpayquick) GetInvoice(invoiceId string) (QpayInvoiceGetResponse, error) {
	var response QpayInvoiceGetResponse
	ctx := q.newContext(context.Background(), "get_invoice", nil, &response, QPayInvoiceGet, invoiceId)
	q.cbs.GetInvoice().Execute(ctx)
	if ctx.Error != nil {
		return QpayInvoiceGetResponse{}, ctx.Error
	}
	return response, nil
}

// CheckPayment [Төлбөр төлөгдсөн эсэхийг шалгах]
func (q *qpayquick) CheckPayment(invoiceID string) (QpayPaymentCheckResponse, error) {
	request := QpayPaymentCheckRequest{InvoiceID: invoiceID}
	var response QpayPaymentCheckResponse
	ctx := q.newContext(context.Background(), "check_payment", request, &response, QPayPaymentCheck, "")
	q.cbs.CheckPayment().Execute(ctx)
	if ctx.Error != nil {
		return QpayPaymentCheckResponse{}, ctx.Error
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
