package qpay_quick

import (
	"context"
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
func (q *qpayquick) httpRequestQPay(goCtx context.Context, body interface{}, result interface{}, api utils.API, urlExt string) error {
	if _, err := q.authQPayV2(goCtx); err != nil {
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
func (q *qpayquick) authQPayV2(goCtx context.Context) (qpayLoginResponse, error) {
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
			res, authErr = q.execRefreshAuth(goCtx, refreshToken)
			if authErr != nil {
				res, authErr = q.execAuth(goCtx)
			}
		} else {
			res, authErr = q.execAuth(goCtx)
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

// execAuth runs the "auth" processor chain and returns the login response.
func (q *qpayquick) execAuth(goCtx context.Context) (qpayLoginResponse, error) {
	var result qpayLoginResponse
	ctx := q.newContext(goCtx, "auth", nil, &result, QPayAuthToken, "")
	q.cbs.Auth().Execute(ctx)
	return result, ctx.Error
}

// execRefreshAuth runs the "refresh_auth" processor chain with the given token.
func (q *qpayquick) execRefreshAuth(goCtx context.Context, refreshToken string) (qpayLoginResponse, error) {
	var result qpayLoginResponse
	ctx := q.newContext(goCtx, "refresh_auth", refreshToken, &result, QPayAuthRefresh, "")
	q.cbs.RefreshAuth().Execute(ctx)
	return result, ctx.Error
}

// tokenValid checks if access token is still valid (must hold mu.RLock)
func (q *qpayquick) tokenValid() bool {
	return time.Now().Before(time.Unix(q.loginObject.ExpiresIn, 0).Add(-1 * time.Minute))
}

// refreshTokenValid checks if refresh token is still valid (must hold mu.RLock)
func (q *qpayquick) refreshTokenValid() bool {
	return time.Now().Before(time.Unix(q.loginObject.RefreshExpiresIn, 0).Add(-1 * time.Minute))
}
