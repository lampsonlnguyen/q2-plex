package q2spot
import ("strings";"testing")
func TestEscapedProtocol(t *testing.T){
 r,err:=Parse([]byte("Q2SPOT/1\n10\talbum\tA%09B\n"))
 if err!=nil||r.Op!="album"||r.Args[0]!="A\tB"{t.Fatalf("parse %+v %v",r,err)}
 out:=string((Reply{Rows:[]Row{{Kind:"info",Title:"Việt Nam\trock"}}}).Encode("10"))
 if !strings.Contains(out,"Việt Nam%09rock"){t.Fatal("escape missing")}
}
func TestRejectBadEncoding(t *testing.T){
 if _,err:=Parse([]byte("Q2SPOT/1\n1\talbum\t%00\n"));err==nil{t.Fatal("accepted NUL")}
}
