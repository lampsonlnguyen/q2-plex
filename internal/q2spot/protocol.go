package q2spot

import (
 "bufio"
 "errors"
 "fmt"
 "io"
 "net"
 "os"
 "strings"
 "time"
 "unicode/utf8"
)

const maxRequest=8<<10
const maxReply=64<<10
const maxRows=50
const maxText=255

type Request struct { ID,Op string; Args []string }
type Row struct { Kind,URI,Title,Subtitle,Context,Position string }
type Reply struct { Status,Message,Cursor string; Rows []Row }
func Fail(s,msg string)Reply{return Reply{Status:s,Message:msg}}
func esc(s string)string{
 return strings.NewReplacer("%","%25","\t","%09","\n","%0A","\r","%0D","\x00","").Replace(s)
}
func unesc(s string)(string,error){
 // Values are opaque except for Q2SPOT/1 escapes.
 var b strings.Builder
 for i:=0;i<len(s);i++{
  if s[i]!='%'{if s[i]==0{return "",errors.New("NUL")};b.WriteByte(s[i]);continue}
  if i+2>=len(s){return "",errors.New("short escape")}
  var n byte
  for _,ch:=range s[i+1:i+3]{
   n*=16
   switch{case ch>='0'&&ch<='9':n+=ch-'0';case ch>='a'&&ch<='f':n+=ch-'a'+10;case ch>='A'&&ch<='F':n+=ch-'A'+10;default:return "",errors.New("invalid escape")}
  }
  if n==0{return "",errors.New("escaped NUL")}
  b.WriteByte(n);i+=2
 }
 return b.String(),nil
}
func Parse(b []byte)(Request,error){
 s:=string(b)
 if len(s)>maxRequest||!strings.HasPrefix(s,"Q2SPOT/1\n"){return Request{},errors.New("bad protocol")}
 line:=strings.TrimSuffix(strings.TrimPrefix(s,"Q2SPOT/1\n"),"\n")
 if strings.ContainsAny(line,"\r\n"){return Request{},errors.New("bad line")}
 f:=strings.Split(line,"\t")
 if len(f)<2{return Request{},errors.New("missing op")}
 for i,v:=range f{u,err:=unesc(v);if err!=nil{return Request{},err};f[i]=u}
 return Request{ID:f[0],Op:f[1],Args:f[2:]},nil
}
func clip(s string,n int)string{
 if len(s)<=n{return s}
 for n>0&&!utf8.RuneStart(s[n]){n--}
 return s[:n]
}
func (r Reply) Encode(id string)[]byte{
 if r.Status==""{r.Status="ok"}
 var b strings.Builder
 b.WriteString("Q2SPOT/1\n")
 b.WriteString(esc(id)+"\t"+esc(r.Status)+"\t"+esc(clip(r.Message,120))+"\t"+esc(r.Cursor)+"\n")
 for i,row:=range r.Rows{
  if i>=maxRows{break}
  line:=strings.Join([]string{esc(row.Kind),esc(row.URI),esc(clip(row.Title,maxText)),esc(clip(row.Subtitle,maxText)),esc(row.Context),esc(row.Position)},"\t")+"\n"
  if b.Len()+len(line)>maxReply{break}
  b.WriteString(line)
 }
 return []byte(b.String())
}
func Serve(socket string,h func(Request)Reply)error{
 _=os.Remove(socket)
 l,err:=net.Listen("unix",socket);if err!=nil{return err}
 defer l.Close()
 defer os.Remove(socket)
 for{
  c,err:=l.Accept();if err!=nil{return err}
  go func(){
   defer c.Close();_=c.SetDeadline(time.Now().Add(15*time.Second))
   // Firmware closes its write half. A capped read prevents unbounded allocations.
   b,err:=io.ReadAll(io.LimitReader(bufio.NewReader(c),maxRequest+1))
   if err!=nil{return}
   req,err:=Parse(b)
   if err!=nil{_,_=io.WriteString(c,string(Fail("bad","Invalid request").Encode("0")));return}
   func(){defer func(){if recover()!=nil{_,_=fmt.Fprint(c,string(Fail("error","Internal error").Encode(req.ID)))}}()
    _,_=c.Write(h(req).Encode(req.ID))
   }()
  }()
 }
}
