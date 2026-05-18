package qpay_v2

import (
	"fmt"
	"net/http"
	"time"

	"github.com/batorgil-it/qpay-go/utils"
)

var (
	// QPayAuthToken [Access Token авах]
	QPayAuthToken = utils.API{
		Url:    "/auth/token",
		Method: http.MethodPost,
	}
	// QPayAuthRefresh [Access Token шинэчлэх]
	QPayAuthRefresh = utils.API{
		Url:    "/auth/refresh",
		Method: http.MethodPost,
	}
	// QPayInvoiceCreate [Нэхэмжлэх үүсгэх]
	QPayInvoiceCreate = utils.API{
		Url:    "/invoice",
		Method: http.MethodPost,
	}
	// QPayInvoiceGet [Нэхэмжлэх харах]
	QPayInvoiceGet = utils.API{
		Url:    "/invoice/",
		Method: http.MethodGet,
	}
	// QPayInvoiceCancel [Нэхэмжлэх цуцлах]
	QPayInvoiceCancel = utils.API{
		Url:    "/invoice/",
		Method: http.MethodDelete,
	}
	// QPayPaymentGet [Төлбөр харах]
	QPayPaymentGet = utils.API{
		Url:    "/payment/",
		Method: http.MethodGet,
	}
	// QPayPaymentCheck [Төлбөр шалгах]
	QPayPaymentCheck = utils.API{
		Url:    "/payment/check",
		Method: http.MethodPost,
	}
	// QPayPaymentCancel [Төлбөр цуцлах]
	QPayPaymentCancel = utils.API{
		Url:    "/payment/cancel/",
		Method: http.MethodDelete,
	}
	// QPayPaymentRefund [Төлбөр буцаах]
	QPayPaymentRefund = utils.API{
		Url:    "/payment/refund/",
		Method: http.MethodDelete,
	}
	// QPayPaymentList [Төлбөрийн жагсаалт]
	QPayPaymentList = utils.API{
		Url:    "/payment/list",
		Method: http.MethodPost,
	}
	// QPayEbarimtCreate [И-баримт үүсгэх]
	QPayEbarimtCreate = utils.API{
		Url:    "/ebarimt_v3/create",
		Method: http.MethodPost,
	}
	// QPayEbarimtCancel [И-баримт цуцлах]
	QPayEbarimtCancel = utils.API{
		Url:    "/ebarimt_v3/",
		Method: http.MethodDelete,
	}
)

// httpRequestQPay [Internal: QPay API-руу HTTP хүсэлт илгээх туслах функц]
// body: Хүсэлтийн бие (POST/PUT үед)
// result: Хариуг задлах бүтэц (struct pointer)
// api: utils.API төрлийн эндпоинт тохиргоо
// urlExt: URL-д залгагдах нэмэлт ID (invoice_id, payment_id г.м)
func (q *qpay) httpRequestQPay(body interface{}, result interface{}, api utils.API, urlExt string) error {

	_, authErr := q.authQPayV2()
	if authErr != nil {
		return authErr
	}

	// Ensure thread safety for token fetch
	q.mu.RLock()
	token := ""
	if q.loginObject != nil {
		token = q.loginObject.AccessToken
	}
	q.mu.RUnlock()

	url := q.endpoint + api.Url + urlExt
	req := q.client.R().
		SetHeader("Content-Type", "application/json").
		SetAuthToken(token).
		SetResult(result)

	// Standard guard: avoid sending identity bodies on non-mutation requests
	if body != nil {
		req.SetBody(body)
	}

	res, err := req.Execute(api.Method, url)

	if err != nil {
		return err
	}

	if res.IsError() {
		return fmt.Errorf("%s-QPay response error: %s (Status: %d)",
			time.Now().Format("2006-01-02 15:04:05"),
			res.String(),
			res.StatusCode())
	}

	return nil
}

// authQPayV2 [Internal: qPay-ээс Access Token авах/шинэчлэх]
// Simple: check token → if valid return cached → if expired, one goroutine auths via singleflight.
// See: https://developer.qpay.mn/#auth-token
func (q *qpay) authQPayV2() (qpayLoginResponse, error) {
	// Fast path: token still valid
	q.mu.RLock()
	if q.loginObject != nil && q.tokenValid() {
		res := *q.loginObject
		q.mu.RUnlock()
		return res, nil
	}
	q.mu.RUnlock()

	// Slow path: singleflight deduplicates concurrent auth calls
	v, err, _ := q.authGroup.Do("auth", func() (any, error) {
		// Double-check after waiting
		q.mu.RLock()
		if q.loginObject != nil && q.tokenValid() {
			res := *q.loginObject
			q.mu.RUnlock()
			return res, nil
		}

		// Try refresh token first, fallback to full auth
		canRefresh := q.loginObject != nil && q.loginObject.RefreshToken != "" && q.refreshTokenValid()
		var refreshToken string
		if canRefresh {
			refreshToken = q.loginObject.RefreshToken
		}
		q.mu.RUnlock()

		var res qpayLoginResponse
		var authErr error
		if canRefresh {
			res, authErr = q.doRefresh(refreshToken)
			if authErr != nil {
				// Refresh failed — fallback to full auth
				res, authErr = q.doAuth()
			}
		} else {
			res, authErr = q.doAuth()
		}
		if authErr != nil {
			return res, authErr
		}

		q.mu.Lock()
		q.loginObject = &res
		q.mu.Unlock()
		return res, nil
	})
	if err != nil {
		return qpayLoginResponse{}, err
	}
	return v.(qpayLoginResponse), nil
}

// tokenValid checks if access token is still valid (must hold mu.RLock)
func (q *qpay) tokenValid() bool {
	return time.Now().Before(time.Unix(q.loginObject.ExpiresIn, 0).Add(-1 * time.Minute))
}

// refreshTokenValid checks if refresh token is still valid (must hold mu.RLock)
func (q *qpay) refreshTokenValid() bool {
	return time.Now().Before(time.Unix(q.loginObject.RefreshExpiresIn, 0).Add(-1 * time.Minute))
}

// doAuth [Full auth: username/password]
func (q *qpay) doAuth() (qpayLoginResponse, error) {
	var authRes qpayLoginResponse
	res, err := q.client.R().
		SetHeader("Content-Type", "application/json").
		SetBasicAuth(q.username, q.password).
		SetResult(&authRes).
		Post(q.endpoint + QPayAuthToken.Url)
	if err != nil {
		return authRes, err
	}
	if res.IsError() {
		return authRes, fmt.Errorf("%s-QPay auth failed: %s (Status: %d)",
			time.Now().Format("2006-01-02 15:04:05"), res.String(), res.StatusCode())
	}
	return authRes, nil
}

// doRefresh [Refresh token ашиглан access token шинэчлэх]
func (q *qpay) doRefresh(refreshToken string) (qpayLoginResponse, error) {
	var authRes qpayLoginResponse
	res, err := q.client.R().
		SetHeader("Content-Type", "application/json").
		SetAuthToken(refreshToken).
		SetResult(&authRes).
		Post(q.endpoint + QPayAuthRefresh.Url)
	if err != nil {
		return authRes, err
	}
	if res.IsError() {
		return authRes, fmt.Errorf("%s-QPay refresh failed: %s (Status: %d)",
			time.Now().Format("2006-01-02 15:04:05"), res.String(), res.StatusCode())
	}
	return authRes, nil
}
