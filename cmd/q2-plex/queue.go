package main

import (
 "context"
 "errors"
 "fmt"
 "net/url"
 "strconv"
 "strings"
 "time"

 "github.com/lampsonlnguyen/q2-plex/internal/plex"
 "github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

const queueLimit = 250
const queueFile = "/tmp/q2-plex.urls"
const playFile = "/tmp/q2-plex.play"

// queue state is small, holds metadata only, and never includes Plex tokens.
type playbackQueue struct {
 Tracks []plex.Item
 At int
}
func cleanField(s string) string {
 s = strings.Map(func(r rune) rune {
  if r < 0x20 || r == 0x7f { return ' ' }
  return r
 }, s)
 if len(s) > 250 { s = string([]rune(s)[:min(len([]rune(s)), 150)]) }
 return s
}
func buildURLs(tracks []plex.Item, start int) ([]byte,error) {
 if start < 0 || start >= len(tracks) || len(tracks) > queueLimit {
  return nil, errors.New("invalid queue index")
 }
 var b strings.Builder
 fmt.Fprintf(&b, "#Q2URLS/1\tstart=%d\n",start)
 for _, it := range tracks {
  // Plex music tracks have numeric rating keys. Keep URLs on loopback; no tokens.
  if it.Type!="track" || !numericID(it.RatingKey) {return nil,errors.New("invalid track in queue")}
  uri:="http://"+relayAddr+"/song/"+url.PathEscape(it.RatingKey)
  fmt.Fprintf(&b,"%s\t%s\t%s\t%s\t\t%d\n",uri,cleanField(it.Title),cleanField(it.GrandparentTitle),cleanField(it.ParentTitle),it.Duration/1000)
 }
 if b.Len() > 256<<10 {return nil,errors.New("queue too large")}
 return []byte(b.String()),nil
}
func numericID(s string) bool {
 if s==""||len(s)>40{return false}
 for _,c:=range s {if c<'0'||c>'9'{return false}}
 return true
}
func (a *app) startQueue(tracks []plex.Item,start int)error{
 data,err:=buildURLs(tracks,start);if err!=nil{return err}
 // The queue file is replaced before the trigger, atomically, so the firmware
 // can never see a partial queue. Native Q2 controls handle later skips.
 if err:=atomic(queueFile,data);err!=nil{return err}
 if err:=atomic(playFile,[]byte(strconv.FormatInt(time.Now().UnixNano(),10)+" "+queueFile));err!=nil{return err}
 a.mu.Lock()
 a.queue=playbackQueue{Tracks:append([]plex.Item(nil),tracks...),At:start}
 a.mu.Unlock()
 return nil
}
func (a *app) playAlbum(ctx context.Context,albumID,selected string)q2spot.Reply{
 if !numericID(albumID) {return q2spot.Fail("bad","Invalid album")}
 tracks,err:=a.client.AlbumTracks(ctx,albumID,queueLimit)
 if err!=nil {return q2spot.Fail("error","Album tracks could not load")}
 if len(tracks)==0{return q2spot.Fail("unavailable","Album has no tracks")}
 start:=0
 if selected!="" {
  start=-1
  for i,item:=range tracks {if item.RatingKey==selected{start=i;break}}
  if start<0 {return q2spot.Fail("unavailable","Track outside album queue")}
 }
 if err:=a.startQueue(tracks,start);err!=nil{return q2spot.Fail("error","Could not start queue")}
 return q2spot.Reply{Message:"Playing "+cleanField(tracks[start].Title)}
}
func (a *app) queuePage()q2spot.Reply{
 a.mu.Lock();q:=a.queue;a.mu.Unlock()
 if len(q.Tracks)==0{return q2spot.Reply{Rows:[]q2spot.Row{{Kind:"info",Title:"No active queue"}}}}
 rows:=make([]q2spot.Row,0,min(len(q.Tracks),40))
 // A bounded first window avoids asking the firmware to store hundreds of rows.
 for i:=0;i<len(q.Tracks)&&i<40;i++ {
  item:=q.Tracks[i]
  rows=append(rows,q2spot.Row{Kind:"action",URI:"playqueue",Title:item.Title,Subtitle:item.GrandparentTitle,Context:strconv.Itoa(i)})
 }
 if len(q.Tracks)>40{rows=append(rows,q2spot.Row{Kind:"info",Title:"Use album view for remaining tracks"})}
 return q2spot.Reply{Rows:rows}
}
func (a *app) playQueueIndex(raw string)q2spot.Reply{
 i,err:=strconv.Atoi(raw)
 a.mu.Lock();q:=a.queue;a.mu.Unlock()
 if err!=nil||i<0||i>=len(q.Tracks){return q2spot.Fail("bad","Invalid queue position")}
 if err:=a.startQueue(q.Tracks,i);err!=nil{return q2spot.Fail("error","Could not start queue")}
 return q2spot.Reply{Message:"Playing "+cleanField(q.Tracks[i].Title)}
}
