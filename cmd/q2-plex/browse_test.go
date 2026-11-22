package main

import (
 "strings"
 "testing"

 "github.com/lampsonlnguyen/q2-plex/internal/plex"
 "github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)
func TestArtHashBasedOnURIAndContext(t *testing.T) {
 a:=artKey(q2spot.Row{URI:"album",Context:"101"})
 b:=artKey(q2spot.Row{URI:"album",Context:"102"})
 if a==b||len(a)!=8{t.Fatalf("bad key a=%s b=%s",a,b)}
}
func TestTrackRowPreservesPlayAndFavoriteHold(t *testing.T){
 row:=musicRow(plex.Item{Type:"track",RatingKey:"44",Title:"Music"},"playlist","22")
 if row.Kind!="action"||row.URI!="play"||row.Context!="pl:22:44"||row.Position!="favtoggle"{t.Fatalf("%+v",row)}
}
func TestQueueIncludesCoverWithoutToken(t *testing.T){
 tracks:=[]plex.Item{{Type:"track",RatingKey:"11",Title:"Song",ParentTitle:"Album",GrandparentTitle:"Artist",Duration:120000}}
 out,err:=buildURLs(tracks,0);if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(out),"/cover/11\t120\n"){t.Fatalf("%s",out)}
 if strings.Contains(string(out),"token"){t.Fatal("unexpected token")}
}
