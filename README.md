# qpay-go

Unofficial Go SDK for the [QPay](https://qpay.mn) payment gateway. Supports QPay V1, QPay V2, and QPay Quick APIs.

## Installation

```bash
go get github.com/batorgil-it/qpay-go
```

**Requires Go 1.23+**

---

## Packages

| Package      | Description                                |
| ------------ | ------------------------------------------ |
| `qpay_v2`    | QPay V2 — recommended for new integrations |
| `qpay_v1`    | QPay V1 — legacy support                   |
| `qpay_quick` | QPay Quick — marketplace/sub-merchant API  |

---

## QPay V2 (Recommended)

### Initialize

```go
import qpay "github.com/batorgil-it/qpay-go/qpay_v2"

client := qpay.New(
    "YOUR_USERNAME",                // QPay username (client_id)
    "YOUR_PASSWORD",                // QPay password (client_secret)
    "https://merchant.qpay.mn/v2", // Production endpoint
    "https://yourapp.com/callback", // Callback base URL
    "YOUR_INVOICE_CODE",            // Invoice code assigned by QPay
    "YOUR_MERCHANT_ID",             // Merchant ID
)
```

**Sandbox endpoint:** `https://merchant-sandbox.qpay.mn/v2`

#### Options

You can pass options to `New()` to customize behavior:

| Option           | Description                                                               |
| ---------------- | ------------------------------------------------------------------------- |
| `WithSyncAuth()` | Block until auth completes (default: async background auth)               |
| `WithClient(c)`  | Inject a custom `resty.Client` (e.g. for custom TLS, proxies, or logging) |

```go
// Sync auth — New() blocks until token is ready
client := qpay.New(
    "USERNAME", "PASSWORD", "ENDPOINT", "CALLBACK", "INVOICE_CODE", "MERCHANT_ID",
    qpay.WithSyncAuth(),
)

// Custom HTTP client
httpClient := resty.New().SetTimeout(15 * time.Second)

client := qpay.New(
    "USERNAME", "PASSWORD", "ENDPOINT", "CALLBACK", "INVOICE_CODE", "MERCHANT_ID",
    qpay.WithClient(httpClient),
)
```

---

### Create Invoice

```go
invoice, err := client.CreateInvoice(qpay.QPayCreateInvoiceInput{
    SenderInvoiceNo:  "INV-2024-001", // Your unique invoice/order number
    InvoiceDescription: "Order #1234",
    Amount:           10000,          // Amount in MNT (integer)
    CallbackParam: map[string]string{
        "order_id": "1234",
    },
    // Advanced B2B fields (optional)
    SenderBranchCode: "BRANCH_01",
    InvoiceDueDate:   "2024-12-31 23:59:59",
})
if err != nil {
    log.Fatal(err)
}

fmt.Println(invoice.InvoiceID)
fmt.Println(invoice.QrImage) // Base64 encoded QR image
fmt.Println(invoice.QrText)  // Raw QR text value
fmt.Println(invoice.Urls)    // Bank app deeplinks
```

### Get Invoice

```go
invoice, err := client.GetInvoice("INVOICE_ID")
if err != nil {
    log.Fatal(err)
}
fmt.Println(invoice.InvoiceStatus) // OPEN, CLOSED, CANCELLED
fmt.Println(invoice.TotalAmount)
```

### Check Payment

```go
result, err := client.CheckPayment("INVOICE_ID", 10, 1) // pageLimit, pageNumber
if err != nil {
    log.Fatal(err)
}

fmt.Println(result.Count)
fmt.Println(result.PaidAmount)

for _, row := range result.Rows {
    fmt.Println(row.PaymentID, row.PaymentStatus, row.PaymentAmount)
}
```

Payment statuses: `NEW`, `PAID`, `FAILED`, `REFUNDED`

### Cancel Invoice

```go
res, err := client.CancelInvoice("INVOICE_ID")
```

### Cancel Payment

```go
res, err := client.CancelPayment("INVOICE_ID", "PAYMENT_ID")
```

### Refund Payment

```go
res, err := client.RefundPayment("INVOICE_ID", "PAYMENT_ID")
```

### Get Payment

```go
payment, err := client.GetPayment("PAYMENT_ID")
```

### Ebarimt 3.0

Use `CreateEbarimtInvoice` when the invoice itself must carry Ebarimt 3.0 tax data. QPay assigns a separate Ebarimt-enabled invoice code for this flow.

```go
invoice, err := client.CreateEbarimtInvoice(qpay.QPayCreateEbarimtInvoiceInput{
    InvoiceCode:         "TEST_EB_INVOICE",
    SenderInvoiceNo:     "TEST_INVOICE_23",
    InvoiceReceiverCode: "23",
    SenderBranchCode:    "TEST_BRANCH",
    InvoiceDescription:  "Test invoice",
    CallbackURL:         "https://example.com/callback",
    TaxType:             qpay.QPayTaxTypeVAT, // "1" taxable, "2" no VAT, "3" exempt
    DistrictCode:        "0101",
    Lines: []*qpay.QPayEbarimtInvoiceLine{
        {
            TaxProductCode:     "",
            LineDescription:    "Улаан буудайн үр",
            LineQuantity:       "1.00",
            LineUnitPrice:      "1000.00",
            Note:               "TEST",
            ClassificationCode: "0111100",
            Taxes: []*qpay.QPayEbarimtTax{
                {
                    TaxCode:     qpay.QPayTaxCodeVAT,
                    Description: "НӨАТ",
                    Amount:      "89.2857",
                    Note:        "НӨАТ",
                },
                {
                    TaxCode:     qpay.QPayTaxCodeCity,
                    Description: "City tax",
                    Amount:      "17.8571",
                    Note:        "TEST",
                },
            },
        },
        {
            TaxProductCode:     "",
            LineDescription:    "Бусад төрлийн сорго будаа",
            LineQuantity:       "1.00",
            LineUnitPrice:      "1000.00",
            Note:               "TEST",
            ClassificationCode: "0114200",
            Taxes: []*qpay.QPayEbarimtTax{
                {
                    TaxCode:     qpay.QPayTaxCodeVAT,
                    Description: "НӨАТ",
                    Amount:      "90.91",
                    Note:        "НӨАТ",
                },
            },
        },
    },
})
```

For `QPayTaxTypeNoVAT` and `QPayTaxTypeVATExempt`, the SDK sends `calculate_vat: false` unless you explicitly override it.

When QPay calls your callback URL, return HTTP `200` with body `SUCCESS`, then call `CheckPayment` with the invoice ID. Do not cron-poll `CheckPayment`.

```go
payment, err := client.CheckPayment(invoice.InvoiceID, 100, 1)
```

If you create Ebarimt after a payment is already paid:

```go
barimt, err := client.CreateEbarimt(qpay.QPayEbarimtCreateInput{
    PaymentID:           "PAYMENT_ID",
    EbarimtReceiverType: qpay.QPayEbarimtReceiverCitizen, // or QPayEbarimtReceiverCompany
    EbarimtReceiver:     "88614450",                      // phone or company register
    DistrictCode:        "3505",
    ClassificationCode:  "0000010",
})
```

Cancel Ebarimt by payment ID:

```go
barimt, err := client.CancelEbarimt("PAYMENT_ID")
```

---

### Token Management

The SDK handles authentication automatically. Tokens are cached and refreshed before expiry — you do not need to manage tokens manually. All methods are safe for concurrent use.

---

## QPay V1

```go
import qpay "github.com/batorgil-it/qpay-go/qpay_v1"

client := qpay.New(
    "CLIENT_ID",
    "CLIENT_SECRET",
    "https://sandbox.qpay.mn/v1", // endpoint
    "https://yourapp.com/callback",
    "MERCHANT_ID",
    "TEMPLATE_ID",
    "BRANCH_ID",
    "POS_ID",
)

// Create invoice
invoice, err := client.CreateInvoice(qpay.QPayInvoiceCreateRequest{
    BillNo:      "ORDER-001",
    Date:        "2024-01-01",
    Description: "Order payment",
    Amount:      10000,
})

// Get invoice
invoice, err := client.GetInvoice("INVOICE_ID")

// Check payment
payment, err := client.CheckPayment("PAYMENT_ID")
```

---

## QPay Quick

QPay Quick is a marketplace API for platforms that onboard sub-merchants.

```go
import qpay "github.com/batorgil-it/qpay-go/qpay_quick"

client := qpay.New(
    "USERNAME",
    "PASSWORD",
    "https://quickpay.qpay.mn", // endpoint
    "https://yourapp.com/callback",
    "INVOICE_CODE",
    "TERMINAL_ID",
)

// Register a company merchant
company, err := client.CreateCompany(qpay.QpayCompanyCreateRequest{
    Name:       "Example LLC",
    RegisterNo: "1234567",
    Phone:      "99001122",
    Email:      "info@example.mn",
    City:       "Ulaanbaatar",
    District:   "Bayanzurkh",
    Address:    "1st khoroo",
    MCCcode:    "5999",
})

// Register a person merchant
person, err := client.CreatePerson(qpay.QpayPersonCreateRequest{
    RegisterNo: "УУ12345678",
    FirstName:  "Bat",
    LastName:   "Bold",
    Phone:      "99001122",
    Email:      "bat@example.mn",
    City:       "Ulaanbaatar",
})

// Get a merchant
merchant, err := client.GetMerchant("MERCHANT_ID")

// List merchants
merchants, err := client.ListMerchant(qpay.QpayOffset{
    PageNumber: 1,
    PageLimit:  20,
})

// Create invoice for a sub-merchant
invoice, err := client.CreateInvoice(qpay.QpayInvoiceRequest{
    MerchantID:  "MERCHANT_ID",
    Amount:      10000,
    Currency:    "MNT",
    Description: "Order payment",
    CallbackUrl: "https://yourapp.com/callback",
})

// Get invoice
invoice, err := client.GetInvoice("INVOICE_ID")

// Check payment
payment, err := client.CheckPayment("INVOICE_ID")
fmt.Println(payment.InvoiceStatus) // OPEN, PAID, CLOSED
```

---

## Error Handling

All methods return a standard Go `error`. On HTTP errors, the error contains the raw response body from QPay.

```go
invoice, err := client.CreateInvoice(input)
if err != nil {
    log.Printf("QPay error: %v", err)
    return
}
```

---

## QPay API Reference

- [QPay V2 Developer Docs](https://developer.qpay.mn)
- [QPay Quick Developer Docs](https://developer.qpay.mn/quick)

---

## License

MIT
