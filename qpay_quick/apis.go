package qpay_quick

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

	// QPayCreateCompany [Байгууллага бүртгэх]
	QPayCreateCompany = utils.API{
		Url:    "/merchant/company",
		Method: http.MethodPost,
	}
	// QPayCreatePerson [Хувь хүн бүртгэх]
	QPayCreatePerson = utils.API{
		Url:    "/merchant/person",
		Method: http.MethodPost,
	}
	// QPayUpdateCompany [Байгууллагаар бүртгэсэн мерчантын мэдээлэл шинэчлэх]
	QPayUpdateCompany = utils.API{
		Url:    "/merchant/company/",
		Method: http.MethodPut,
	}
	// QPayUpdatePerson [Хувь хүнээр бүртгэсэн мерчантын мэдээлэл шинэчлэх]
	QPayUpdatePerson = utils.API{
		Url:    "/merchant/person/",
		Method: http.MethodPut,
	}
	// QPayGetMerchant [Мерчантын мэдээлэл харах]
	QPayGetMerchant = utils.API{
		Url:    "/merchant/",
		Method: http.MethodGet,
	}
	// QPayDeleteMerchant [Мерчантыг устгах]
	QPayDeleteMerchant = utils.API{
		Url:    "/merchant/",
		Method: http.MethodDelete,
	}
	// QPayMerchantList [Мерчантуудын жагсаалт]
	QPayMerchantList = utils.API{
		Url:    "/merchant/list",
		Method: http.MethodPost,
	}
	// QPayGetAimagHot [Аймаг/хотын код жагсаалт]
	QPayGetAimagHot = utils.API{
		Url:    "/aimaghot",
		Method: http.MethodGet,
	}
	// QPayGetSumDuureg [Сум/дүүргийн код жагсаалт]
	QPayGetSumDuureg = utils.API{
		Url:    "/sumduureg/",
		Method: http.MethodGet,
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

	// QPayPaymentCheck [Төлбөр шалгах]
	QPayPaymentCheck = utils.API{
		Url:    "/payment/check",
		Method: http.MethodPost,
	}
)

// httpRequestQPay [Internal: QPay API-руу HTTP хүсэлт илгээх туслах функц]
// body: Хүсэлтийн бие (POST/PUT үед)
// result: Хариуг задлах бүтэц (struct pointer)
// api: utils.API төрлийн эндпоинт тохиргоо
// urlExt: URL-д залгагдах нэмэлт ID
func (q *qpayquick) httpRequestQPay(body interface{}, result interface{}, api utils.API, urlExt string) error {
	if _, err := q.authQPayV2(); err != nil {
		return err
	}

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
func (q *qpayquick) authQPayV2() (qpayLoginResponse, error) {
	q.mu.RLock()
	if q.loginObject != nil && q.tokenValid() {
		res := *q.loginObject
		q.mu.RUnlock()
		return res, nil
	}
	q.mu.RUnlock()

	v, err, _ := q.authGroup.Do("auth", func() (any, error) {
		q.mu.RLock()
		if q.loginObject != nil && q.tokenValid() {
			res := *q.loginObject
			q.mu.RUnlock()
			return res, nil
		}

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
func (q *qpayquick) tokenValid() bool {
	return time.Now().Before(time.Unix(q.loginObject.ExpiresIn, 0).Add(-1 * time.Minute))
}

// refreshTokenValid checks if refresh token is still valid (must hold mu.RLock)
func (q *qpayquick) refreshTokenValid() bool {
	return time.Now().Before(time.Unix(q.loginObject.RefreshExpiresIn, 0).Add(-1 * time.Minute))
}

// doAuth [Full auth: username/password + terminal_id]
func (q *qpayquick) doAuth() (qpayLoginResponse, error) {
	var authRes qpayLoginResponse
	res, err := q.client.R().
		SetHeader("Content-Type", "application/json").
		SetBasicAuth(q.username, q.password).
		SetBody(map[string]string{"terminal_id": q.terminalID}).
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
func (q *qpayquick) doRefresh(refreshToken string) (qpayLoginResponse, error) {
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
