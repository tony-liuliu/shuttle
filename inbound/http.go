package inbound

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/sipt/shuttle/constant"
	"github.com/sipt/shuttle/constant/typ"
	"github.com/sipt/shuttle/listener"
	"github.com/sirupsen/logrus"

	connpkg "github.com/sipt/shuttle/conn"
)

const (
	ParamsKeyAuthType = "auth_type"

	AuthTypeBasic  = "basic"
	AuthTypeBearer = "bearer"

	ParamsKeyUser     = "user"
	ParamsKeyPassword = "password"
	ParamsKeyToken    = "token"
)

func init() {
	Register("http", newHTTPInbound)
}

func newHTTPInbound(addr string, params map[string]string) (listen func(context.Context, typ.HandleFunc), err error) {
	authType, ok := params[ParamsKeyAuthType]
	authFunc := func(r *http.Request) bool { return true }
	if ok {
		switch authType {
		case AuthTypeBasic:
			authFunc, err = newBasicAuth(params)
		case AuthTypeBearer:
			authFunc, err = newBearerAuth(params)
		default:
			err = errors.Errorf("[http.Inbound] is not support")
		}
		if err != nil {
			return
		}
	}
	dial, err := listener.Get("tcp", addr)
	if err != nil {
		return nil, err
	}
	logrus.WithField("addr", "http://"+addr).Info("http listen starting")
	return func(ctx context.Context, handle typ.HandleFunc) {
		dial(ctx, newHttpHandleFunc(authFunc, handle))
	}, nil
}

func newHttpHandleFunc(authFunc func(r *http.Request) bool, handle typ.HandleFunc) func(conn connpkg.ICtxConn) {
	if authFunc == nil {
		authFunc = func(r *http.Request) bool { return true }
	}
	return func(conn connpkg.ICtxConn) {
		defer conn.Close()
		for {
			req, err := http.ReadRequest(bufio.NewReader(conn))
			if err != nil {
				if err == io.EOF {
					return
				}
				logrus.WithError(err).Error("[http.Inbound] parse to http request failed")
				return
			}
			logrus.WithField("conn-id", conn.GetConnID()).WithField("host", req.Host).
				Debug("start dial HTTP/HTTPS connection")
			if !authFunc(req) {
				resp := &http.Response{
					StatusCode: http.StatusProxyAuthRequired,
				}
				err = resp.Write(conn)
				if err != nil {
					logrus.WithError(err).Error("[http.Inbound] write to response failed")
					return
				}
				return
			}
			if req.Method == http.MethodConnect {
				conn.WithValue(constant.KeyProtocol, "https")
				c, err := httpsHandshake(req, conn)
				if err != nil {
					logrus.WithError(err).Error("[http.Inbound] https handshake failed")
					return
				}
				handle(c)
				return
			} else {
				conn.WithValue(constant.KeyProtocol, "http")
				c, err := httpHandshake(req, conn)
				if err != nil {
					logrus.WithError(err).Error("[http.Inbound] http handshake failed")
					return
				}
				go handle(c)
			}
		}
	}
}

func newBasicAuth(params map[string]string) (func(*http.Request) bool, error) {
	user := params[ParamsKeyUser]
	if len(user) == 0 {
		return nil, errors.New("[user] is empty")
	}
	password := params[ParamsKeyPassword]
	if len(password) == 0 {
		return nil, errors.New("[password] is empty")
	}
	authorization := "Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", user, password)))
	return func(req *http.Request) bool {
		return req.Header.Get("Proxy-Authorization") == authorization
	}, nil
}

func newBearerAuth(params map[string]string) (func(*http.Request) bool, error) {
	token := params[ParamsKeyToken]
	if len(token) == 0 {
		return nil, errors.New("[token] is empty")
	}
	authorization := "Bearer " + token
	return func(req *http.Request) bool {
		return req.Header.Get("Proxy-Authorization") == authorization
	}, nil
}

func httpHandshake(req *http.Request, c connpkg.ICtxConn) (connpkg.ICtxConn, error) {
	ctxReq := &request{
		network: "tcp",
		uri:     req.URL.String(),
		domain:  req.URL.Hostname(),
	}
	var err error
	if port := req.URL.Port(); port != "" {
		ctxReq.port, err = strconv.Atoi(port)
		if err != nil {
			return nil, errors.Errorf("port [%s] is error: %s", port, err.Error())
		}
	} else {
		ctxReq.SetPort(80)
	}
	filterProxyHeader(req)
	hc := &httpConn{
		req:      req,
		ICtxConn: connpkg.NewConn(c, context.WithValue(c, constant.KeyRequestInfo, ctxReq)),
		Mutex:    &sync.Mutex{},
	}
	return hc, nil
}

func filterProxyHeader(req *http.Request) {
	for k := range req.Header {
		if strings.HasPrefix(k, "Proxy") {
			req.Header.Del(k)
		}
	}
}

func httpsHandshake(req *http.Request, c connpkg.ICtxConn) (connpkg.ICtxConn, error) {
	_, err := c.Write([]byte(fmt.Sprintf("%s 200 Connection established\r\n\r\n", req.Proto)))
	if err != nil {
		return nil, errors.Wrapf(err, "https handshake failed")
	}
	ctxReq := &request{
		network: "tcp",
		uri:     req.URL.String(),
		domain:  req.URL.Hostname(),
	}
	if port := req.URL.Port(); port != "" {
		ctxReq.port, err = strconv.Atoi(port)
		if err != nil {
			return nil, errors.Errorf("port [%s] is error: %s", port, err.Error())
		}
	} else {
		ctxReq.SetPort(443)
	}
	c.WithValue(constant.KeyRequestInfo, ctxReq)
	return c, nil
}

type httpConn struct {
	req *http.Request
	connpkg.ICtxConn
	*sync.Mutex
}

func (h *httpConn) WriteTo(w io.Writer) (n int64, err error) {
	wr := &writer{Writer: w}
	err = h.req.Write(wr)
	n = wr.length
	return n, err
}

func (h *httpConn) Read(b []byte) (int, error) {
	return 0, errors.New("httpConn not support read")
}

func (h *httpConn) Close() error {
	return h.req.Body.Close()
}

type writer struct {
	length int64
	io.Writer
}

func (w *writer) Write(b []byte) (n int, err error) {
	n, err = w.Writer.Write(b)
	w.length += int64(n)
	return n, err
}

type request struct {
	network     string
	domain      string
	uri         string
	ip          net.IP
	port        int
	countryCode string
}

func (r *request) Network() string {
	return r.network
}
func (r *request) Domain() string {
	return r.domain
}
func (r *request) URI() string {
	return r.uri
}
func (r *request) IP() net.IP {
	return r.ip
}
func (r *request) CountryCode() string {
	return r.countryCode
}
func (r *request) Port() int {
	return r.port
}
func (r *request) SetIP(in net.IP) {
	r.ip = in
}
func (r *request) SetPort(in int) {
	r.port = in
}
func (r *request) SetCountryCode(in string) {
	r.countryCode = in
}