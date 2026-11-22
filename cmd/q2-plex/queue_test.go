package main

import (
 "strings"
 "testing"

 "github.com/lampsonlnguyen/q2-plex/internal/plex"
)
func TestQueueStartFromTrack(t *testing.T){
 tracks:=[]plex.Item{
  {Type:"track",RatingKey:"123",Title:"One",ParentTitle:"Album",GrandparentTitle:"Artist",Duration:75000},
  {Type:"track",RatingKey:"456",Title:"Two",ParentTitle:"Album",GrandparentTitle:"Artist",Duration:92000},
 }
 data,err:=buildURLs(tracks,1);if err!=nil{t.Fatal(err)}
 s:=string(data)
 if !strings.HasPrefix(s,"#Q2URLS/1\tstart=1\n"){t.Fatalf("start %s",s)}
 if !strings.Contains(s,"http://127.0.0.1:2328/song/456\tTwo\tArtist\tAlbum\t\t92\n"){t.Fatal(s)}
 if strings.Contains(s,"X-Plex-Token"){t.Fatal("token leaked")}
}
func TestQueueRejectsBadTrack(t *testing.T){
 if _,err:=buildURLs([]plex.Item{{Type:"movie",RatingKey:"1"}},0);err==nil{t.Fatal("accepted video")}
 if _,err:=buildURLs([]plex.Item{{Type:"track",RatingKey:"../foo"}},0);err==nil{t.Fatal("accepted bad ID")}
 if _,err:=buildURLs([]plex.Item{{Type:"track",RatingKey:"1"}},1);err==nil{t.Fatal("accepted bad index")}
}
