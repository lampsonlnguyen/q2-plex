package plex

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)
func TestRetryGetOnlyOnTransientFailure(t *testing.T){
	calls:=0
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		calls++
		if calls==1{http.Error(w,"temporarily unavailable",http.StatusServiceUnavailable);return}
		_,_=fmt.Fprint(w,`{"ok":true}`)
	}))
	defer server.Close()
	c,err:=Load(t.TempDir());if err!=nil{t.Fatal(err)}
	var result struct{Ok bool `json:"ok"`}
	if err=c.Request(context.Background(),"GET",server.URL,"/health","",nil,&result);err!=nil{t.Fatal(err)}
	if !result.Ok||calls!=2{t.Fatalf("GET was not retried: %+v calls=%d",result,calls)}
}
func TestWriteAndAuthErrorsNeverRetried(t *testing.T){
	for _,method:=range []string{http.MethodPost,http.MethodPut}{
		calls:=0
		server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
			calls++;http.Error(w,"bad gateway",502)
		}))
		c,err:=Load(t.TempDir());if err!=nil{t.Fatal(err)}
		err=c.Request(context.Background(),method,server.URL,"/timeline","",nil,nil)
		server.Close()
		if err==nil||calls!=1{t.Fatalf("%s replayed %d times; err %v",method,calls,err)}
	}
	calls:=0
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		calls++;http.Error(w,"unauthorized",401)
	}))
	defer server.Close()
	c,err:=Load(t.TempDir());if err!=nil{t.Fatal(err)}
	if err=c.Request(context.Background(),"GET",server.URL,"/library","",nil,nil);err==nil||calls!=1{
		t.Fatalf("unauthorized GET retried %d times, err=%v",calls,err)
	}
}
func TestCertificateErrorsNeverRetried(t *testing.T){
	err:=&url.Error{Op:"Get",URL:"https://plex.invalid",Err:x509.UnknownAuthorityError{}}
	if Retryable(err){t.Fatal("certificate error must not be retried")}
	if !Retryable(&HTTPStatusError{Code:503}){t.Fatal("transient 503 expected")}
	if Retryable(&HTTPStatusError{Code:404}){t.Fatal("404 is permanent")}
	if !Retryable(context.DeadlineExceeded){t.Fatal("transient timeout")}
	if Retryable(errors.New("invalid track")){t.Fatal("permanent validation error")}
}
func TestProbeSavedServerWithoutCloudLookup(t *testing.T){
	calls:=0
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		calls++
		if r.URL.Path!="/library/sections"{t.Fatalf("unexpected cloud or API path: %s",r.URL.Path)}
		_,_=fmt.Fprint(w,`{"MediaContainer":{"Directory":[{"key":"9","title":"Music","type":"artist"}]}}`)
	}))
	defer server.Close()
	c,err:=Load(t.TempDir());if err!=nil{t.Fatal(err)}
	c.State.ServerURL=server.URL;c.State.ServerToken="token";c.State.LibraryID="9"
	if err=c.ProbeSavedServer(context.Background());err!=nil{t.Fatal(err)}
	if calls!=1||c.State.LibraryID!="9"{t.Fatalf("unnecessary calls/changed library: %d %s",calls,c.State.LibraryID)}
}
