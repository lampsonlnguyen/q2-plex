package plex

import (
 "crypto/ed25519"
 "encoding/base64"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestAllowedRoot(t *testing.T) {
 for _,tc:=range []struct{raw string;ok bool}{
  {"https://myserver.plex.direct:32400",true},
  {"http://192.168.5.215:32400",true},
  {"http://127.0.0.1:32400",true},
  {"http://example.com:32400",false},
  {"file:///etc/passwd",false},
  {"http://8.8.8.8:32400",false},
 } {if got:=allowedRoot(tc.raw);got!=tc.ok{t.Errorf("%s: got %t",tc.raw,got)}}
}
func TestPINAndDeviceJWT(t *testing.T){
 c,err:=Load(t.TempDir());if err!=nil{t.Fatal(err)}
 s:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("X-Plex-Client-Identifier")!=c.State.ClientID{t.Error("missing client identifier")}
  if r.Method=="POST"{_,_=w.Write([]byte(`{"id":123,"code":"1234"}`));return}
  if r.URL.Query().Get("deviceJWT")==""{t.Error("missing device JWT")}
  _,_=w.Write([]byte(`{"authToken":"test-token"}`))
 }))
 defer s.Close();c.Cloud=s.URL
 pin,err:=c.StartPIN(t.Context());if err!=nil{t.Fatal(err)}
 if pin.Code!="1234"{t.Fatalf("code=%s",pin.Code)}
 token,err:=c.PollPIN(t.Context(),pin.ID);if err!=nil{t.Fatal(err)}
 if token!="test-token"{t.Fatal("no token")}
 jwt,err:=c.jwt("abc");if err!=nil{t.Fatal(err)}
 parts:=strings.Split(jwt,".");if len(parts)!=3{t.Fatal("bad JWT")}
 data,err:=base64.RawURLEncoding.DecodeString(parts[1]);if err!=nil{t.Fatal(err)}
 var claims map[string]any;_ =json.Unmarshal(data,&claims)
 if claims["nonce"]!="abc"{t.Fatal("missing nonce")}
 priv,err:=c.private();if err!=nil{t.Fatal(err)}
 sig,_:=base64.RawURLEncoding.DecodeString(parts[2])
 if !ed25519.Verify(priv.Public().(ed25519.PublicKey),[]byte(parts[0]+"."+parts[1]),sig){t.Fatal("bad signature")}
}
