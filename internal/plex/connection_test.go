package plex

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestDirectConnectSelectsMusicAndUsesAccountToken(t *testing.T) {
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/library/sections" {t.Errorf("unexpected path %s",r.URL.Path)}
  if r.Header.Get("X-Plex-Token")!="account-token" {t.Error("missing account token")}
  fmt.Fprint(w,`{"MediaContainer":{"Directory":[{"type":"movie","key":"2","title":"Films"},{"type":"artist","key":"7","title":"Music"}]}}`)
 }))
 defer srv.Close()
 c,err:=Load(t.TempDir());if err!=nil{t.Fatal(err)}
 c.State.Token="account-token"
 if err=c.ConnectDirect(context.Background(),srv.URL);err!=nil{t.Fatal(err)}
 if c.State.LibraryID!="7"||c.State.ServerURL!=srv.URL{t.Fatalf("wrong library %s server %s",c.State.LibraryID,c.State.ServerURL)}
 saved,err:=Load(c.Home);if err!=nil{t.Fatal(err)}
 if saved.State.LibraryID!="7"{t.Fatal("selection not persisted")}
}
func TestDirectConnectRejectsInsecureRemoteAndVideoOnly(t *testing.T) {
 c,err:=Load(t.TempDir());if err!=nil{t.Fatal(err)}
 c.State.Token="account-token"
 if err:=c.ConnectDirect(context.Background(),"http://example.com:32400");err==nil{t.Fatal("accepted public plaintext")}
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  fmt.Fprint(w,`{"MediaContainer":{"Directory":[{"type":"movie","key":"9","title":"Video"}]}}`)
 }))
 defer srv.Close()
 if err:=c.ConnectDirect(context.Background(),srv.URL);err==nil||!strings.Contains(err.Error(),"no Plex music"){t.Fatalf("expected no music error, got %v",err)}
}
