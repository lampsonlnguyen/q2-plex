package plex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"crypto/x509"
	"time"
)

// HTTPStatusError preserves the status code so clients can distinguish a
// temporarily unavailable Plex server from bad credentials or broken data.
type HTTPStatusError struct{Code int}
func (e *HTTPStatusError) Error()string{return fmt.Sprintf("plex HTTP %d",e.Code)}

// Retryable is deliberately conservative. TLS certificate failures, HTTP 401,
// not-found media and malformed metadata are permanent errors, not retry loops.
func Retryable(err error)bool{
	if err==nil{return false}
	if errors.Is(err,context.DeadlineExceeded)||errors.Is(err,io.ErrUnexpectedEOF)||errors.Is(err,io.EOF){return true}
	// url.Error satisfies net.Error even when its cause is a permanent
	// certificate rejection. Unwrap before testing transient network errors.
	var urlErr *url.Error
	if errors.As(err,&urlErr){err=urlErr.Err}
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	if errors.As(err,&unknown)||errors.As(err,&invalid)||errors.As(err,&hostname){
		return false
	}
	var netErr net.Error
	if errors.As(err,&netErr){return true}
	var status *HTTPStatusError
	if errors.As(err,&status){
		switch status.Code{case 429,500,502,503,504:return true}
	}
	return false
}
func waitRetry(ctx context.Context,d time.Duration)error{
	t:=time.NewTimer(d);defer t.Stop()
	select{case <-ctx.Done():return ctx.Err();case <-t.C:return nil}
}
