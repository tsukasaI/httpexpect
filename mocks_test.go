package httpexpect

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type mockRequestFactory struct {
	lastreq *http.Request
	fail    bool
}

func (f *mockRequestFactory) NewRequest(
	method, urlStr string, body io.Reader) (*http.Request, error) {
	if f.fail {
		return nil, errors.New("testRequestFactory")
	}
	f.lastreq = httptest.NewRequest(method, urlStr, body)
	return f.lastreq, nil
}

type mockClient struct {
	req  *http.Request
	resp http.Response
	err  error
	cb   func(req *http.Request) // callback in .Do
}

func (c *mockClient) Do(req *http.Request) (*http.Response, error) {
	defer func() {
		if c.cb != nil {
			c.cb(req)
		}
	}()
	c.req = req
	if c.err == nil {
		c.resp.Header = c.req.Header
		c.resp.Body = c.req.Body
		return &c.resp, nil
	}
	return nil, c.err
}

// mockTransportRedirect mocks a transport that implements RoundTripper
//
// When tripCount < maxRedirect,
// mockTransportRedirect responses with redirectHTTPStatusCode
//
// When tripCount = maxRedirect,
// mockTransportRedirect responses with HTTP 200 OK
type mockTransportRedirect struct {
	// assertFn asserts the HTTP request
	assertFn func(*http.Request)

	// redirectHTTPStatusCode indicates the HTTP status code of redirection response
	redirectHTTPStatusCode int

	// tripCount tracks the number of trip that has been done
	tripCount int

	// maxRedirect indicates the number of trip that can be done for redirection.
	// -1 means always redirect.
	maxRedirect int
}

func newMockTransportRedirect() *mockTransportRedirect {
	return &mockTransportRedirect{
		assertFn:               nil,
		redirectHTTPStatusCode: http.StatusPermanentRedirect,
		maxRedirect:            -1,
	}
}

func (mt *mockTransportRedirect) RoundTrip(origReq *http.Request) (
	*http.Response, error,
) {
	mt.tripCount++

	if mt.assertFn != nil {
		mt.assertFn(origReq)
	}

	res := httptest.NewRecorder()

	if mt.maxRedirect == -1 || mt.tripCount <= mt.maxRedirect {
		res.Result().StatusCode = mt.redirectHTTPStatusCode
		res.Result().Header.Set("Location", "/redirect")
	} else {
		res.Result().StatusCode = http.StatusOK
	}

	return res.Result(), nil
}

type mockQueryEncoder string

// EncodeValues implements query.Encoder.EncodeValues
func (m mockQueryEncoder) EncodeValues(key string, v *url.Values) error {
	if m == "err" {
		return errors.New("encoding error")
	}
	v.Set(key, string(m))
	return nil
}

type mockBody struct {
	reader io.Reader

	readCount int
	readErr   error

	closeCount int
	closeErr   error
}

func newMockBody(body string) *mockBody {
	return &mockBody{
		reader: bytes.NewBufferString(body),
	}
}

func (b *mockBody) Read(p []byte) (int, error) {
	b.readCount++
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}

func (b *mockBody) Close() error {
	b.closeCount++
	if b.closeErr != nil {
		return b.closeErr
	}
	return nil
}

func newMockConfig(r Reporter) Config {
	return Config{Reporter: r}.withDefaults()
}

func newMockChain(t *testing.T) *chain {
	return newChainWithDefaults("test", newMockReporter(t))
}

func newFailedChain(t *testing.T) *chain {
	chain := newMockChain(t)
	chain.setFlags(flagFailed)
	return chain
}

type mockLogger struct {
	testing     *testing.T
	logged      bool
	lastMessage string
}

func newMockLogger(t *testing.T) *mockLogger {
	return &mockLogger{testing: t}
}

func (l *mockLogger) Logf(message string, args ...interface{}) {
	l.testing.Logf(message, args...)
	l.lastMessage = fmt.Sprintf(message, args...)
	l.logged = true
}

type mockReporter struct {
	testing  *testing.T
	reported bool
	reportCb func()
}

func newMockReporter(t *testing.T) *mockReporter {
	return &mockReporter{testing: t}
}

func (r *mockReporter) Errorf(message string, args ...interface{}) {
	r.testing.Logf("Fail: "+message, args...)
	r.reported = true

	if r.reportCb != nil {
		r.reportCb()
	}
}

type mockFormatter struct {
	testing          *testing.T
	formattedSuccess int
	formattedFailure int
}

func newMockFormatter(t *testing.T) *mockFormatter {
	return &mockFormatter{testing: t}
}

func (f *mockFormatter) FormatSuccess(ctx *AssertionContext) string {
	f.formattedSuccess++
	return ctx.TestName
}

func (f *mockFormatter) FormatFailure(
	ctx *AssertionContext, failure *AssertionFailure,
) string {
	f.formattedFailure++
	return ctx.TestName
}

type mockAssertionHandler struct {
	ctx     *AssertionContext
	failure *AssertionFailure
}

func (h *mockAssertionHandler) Success(ctx *AssertionContext) {
	h.ctx = ctx
}

func (h *mockAssertionHandler) Failure(
	ctx *AssertionContext, failure *AssertionFailure,
) {
	h.ctx = ctx
	h.failure = failure
}

type mockPrinter struct {
	reqBody  []byte
	respBody []byte
	rtt      time.Duration
}

func (p *mockPrinter) Request(req *http.Request) {
	if req.Body != nil {
		p.reqBody, _ = ioutil.ReadAll(req.Body)
		req.Body.Close()
	}
}

func (p *mockPrinter) Response(resp *http.Response, rtt time.Duration) {
	if resp.Body != nil {
		p.respBody, _ = ioutil.ReadAll(resp.Body)
		resp.Body.Close()
	}
	p.rtt = rtt
}

type mockWebsocketPrinter struct {
	isWrittenTo bool
	isReadFrom  bool
}

func newMockWsPrinter() *mockWebsocketPrinter {
	return &mockWebsocketPrinter{
		isWrittenTo: false,
		isReadFrom:  false,
	}
}

func (p *mockWebsocketPrinter) Request(*http.Request) {
}

func (p *mockWebsocketPrinter) Response(*http.Response, time.Duration) {
}

func (p *mockWebsocketPrinter) WebsocketWrite(typ int, content []byte, closeCode int) {
	p.isWrittenTo = true
}

func (p *mockWebsocketPrinter) WebsocketRead(typ int, content []byte, closeCode int) {
	p.isReadFrom = true
}

type mockWebsocketConn struct {
	subprotocol  string
	closeError   error
	readMsgErr   error
	writeMsgErr  error
	readDlError  error
	writeDlError error
	msgType      int
	msg          []byte
}

func (wc *mockWebsocketConn) Subprotocol() string {
	return wc.subprotocol
}

func (wc *mockWebsocketConn) Close() error {
	return wc.closeError
}

func (wc *mockWebsocketConn) SetReadDeadline(t time.Time) error {
	return wc.readDlError
}

func (wc *mockWebsocketConn) SetWriteDeadline(t time.Time) error {
	return wc.writeDlError
}

func (wc *mockWebsocketConn) ReadMessage() (messageType int, p []byte, err error) {
	return wc.msgType, []byte{}, wc.readMsgErr
}

func (wc *mockWebsocketConn) WriteMessage(messageType int, data []byte) error {
	return wc.writeMsgErr
}

type mockNetError struct {
	isTimeout   bool
	isTemporary bool
}

func (e *mockNetError) Error() string {
	return "mock net error"
}

func (e *mockNetError) Timeout() bool {
	return e.isTimeout
}

func (e *mockNetError) Temporary() bool {
	return e.isTemporary
}

type mockError struct{}

func (e *mockError) Error() string {
	return ""
}
