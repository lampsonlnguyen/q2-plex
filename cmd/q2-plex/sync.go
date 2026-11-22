package main

import (
 "context"
 "errors"
 "fmt"
 "log"
 "os"
 "path/filepath"
 "strings"
 "time"

 "github.com/lampsonlnguyen/q2-plex/internal/offline"
 "github.com/lampsonlnguyen/q2-plex/internal/playback"
 "github.com/lampsonlnguyen/q2-plex/internal/plex"
 "github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

const statusPath="/tmp/q2-plex.now"
const favoritesPath="fav.log"

type playbackSync struct {
 store *playback.Store
 root string
 previous *playback.Now
 previousTrack string
 lastReport time.Time
}

func newPlaybackSync(appDir string)(*playbackSync,error){
 store,err:=playback.Open(filepath.Join(appDir,"last-queue.json"))
 if err!=nil{return nil,err}
 return &playbackSync{store:store,root:appDir},nil
}
func (a *app) loadSavedQueue(){
 if a.syncer==nil{return}
 s:=a.syncer.store.Snapshot()
 if len(s.Tracks)==0{return}
 items:=make([]plex.Item,0,len(s.Tracks))
 local:=map[string]string{}
 for _,t:=range s.Tracks {
  item:=plex.Item{Type:"track",RatingKey:t.ID,Title:t.Title,ParentTitle:t.Album,GrandparentTitle:t.Artist,Duration:t.Duration}
  if t.Favorite{
   v:=10.0;item.UserRating=&v
  }
  items=append(items,item)
  if t.Local!=""{local[t.ID]=t.Local}
 }
 a.mu.Lock()
 a.queue=playbackQueue{Tracks:items,At:s.Index,Local:local}
 a.mu.Unlock()
}
func (a *app) rememberQueue(items []plex.Item,index int,local map[string]string,seq string)error{
 if a.syncer==nil{return nil}
 saved:=playback.Queue{Version:1,Seq:seq,Index:index,Server:a.client.State.ServerName}
 for _,it:=range items{
  tr:=playback.Track{ID:it.RatingKey,Title:it.Title,Artist:it.GrandparentTitle,Album:it.ParentTitle,Duration:it.Duration}
  if it.UserRating!=nil&&*it.UserRating>=9.5{tr.Favorite=true}
  tr.Local=local[it.RatingKey]
  saved.Tracks=append(saved.Tracks,tr)
 }
 return a.syncer.store.Replace(saved)
}
func (a *app) lastQueuePage()q2spot.Reply{
 if a.syncer==nil{return q2spot.Reply{Rows:[]q2spot.Row{text("No saved playback queue")}}}
 s:=a.syncer.store.Snapshot()
 if len(s.Tracks)==0{return q2spot.Reply{Rows:[]q2spot.Row{text("No saved playback queue")}}}
 item:=s.Tracks[s.Index]
 return q2spot.Reply{Rows:[]q2spot.Row{
  text("Last played: "+item.Title),
  text("Artist: "+item.Artist),
  text(fmt.Sprintf("Saved at %d:%02d",s.Seconds/60,s.Seconds%60)),
  text("Restores track order; song starts at beginning"),
  act("resumequeue","Restore Last Queue", ""),
  act("clearlastqueue","Forget Last Queue",""),
 }}
}
func (a *app) resumeQueue()q2spot.Reply{
 if a.syncer==nil{return q2spot.Fail("error","Saved queue unavailable")}
 s:=a.syncer.store.Snapshot()
 if len(s.Tracks)==0{return q2spot.Fail("unavailable","No saved queue")}
 if s.Server!=""&&a.client.State.ServerName!=s.Server {
  allLocal:=true
  for _,t:=range s.Tracks{if t.Local==""{allLocal=false;break}}
  if !allLocal{return q2spot.Fail("offline","Connect to the previous Plex server first")}
 }
 items:=make([]plex.Item,0,len(s.Tracks))
 local:=map[string]string{}
 for _,tr:=range s.Tracks {
  it:=plex.Item{Type:"track",RatingKey:tr.ID,Title:tr.Title,GrandparentTitle:tr.Artist,ParentTitle:tr.Album,Duration:tr.Duration}
  if tr.Favorite{v:=10.0;it.UserRating=&v}
  if tr.Local!="" {
   if a.offline==nil||!a.offline.Exists(offlineTrack(tr)){
    return q2spot.Fail("unavailable","A saved audio file is missing")
   }
   local[tr.ID]=tr.Local
  }
  items=append(items,it)
 }
 if err:=a.startQueueWithLocal(items,s.Index,local);err!=nil{return q2spot.Fail("error","Could not restore queue")}
 return q2spot.Reply{Message:"Queue restored; last song starts from 0:00"}
}
func offlineTrack(t playback.Track)offline.Track {
 return offline.Track{ID:t.ID,File:filepath.Base(t.Local)}
}
func (a *app) handleSync(req q2spot.Request)(q2spot.Reply,bool){
 switch req.Op{
 case "lastqueue":return a.lastQueuePage(),true
 case "resumequeue":return a.resumeQueue(),true
 case "clearlastqueue":
  if a.syncer==nil{return q2spot.Fail("error","Saved queue unavailable"),true}
  if err:=a.syncer.store.Clear();err!=nil{return q2spot.Fail("error","Could not forget queue"),true}
  a.mu.Lock();a.queue=playbackQueue{};a.mu.Unlock()
  return q2spot.Reply{Message:"Saved queue forgotten"},true
 }
 return q2spot.Reply{},false
}
func (a *app) watchPlayback(ctx context.Context) {
 ticker:=time.NewTicker(3*time.Second)
 defer ticker.Stop()
 for {
  a.syncOnce(ctx)
  select {case <-ctx.Done():return;case <-ticker.C:}
 }
}
func (a *app) syncOnce(ctx context.Context){
 if a.syncer==nil{return}
 data,err:=os.ReadFile(statusPath)
 if err==nil {
  now,parseErr:=playback.ParseNow(data)
  if parseErr==nil {a.observeNow(ctx,now)}
 }
 a.flushFavorites(ctx)
}
func (a *app) observeNow(ctx context.Context,now playback.Now){
 s:=a.syncer.store.Snapshot()
 if len(s.Tracks)==0||now.Seq!=s.Seq||now.Index>=len(s.Tracks){return}
 prev:=a.syncer.previous
 // Persist only meaningful changes. The firmware writes position at most every
 // five seconds; this does not poll the SD card or write audio caches.
 if prev==nil||prev.Seq!=now.Seq||prev.Index!=now.Index||prev.State!=now.State||
  now.Seconds>=s.Seconds+5 || now.Seconds<s.Seconds {
  if err:=a.syncer.store.Update(now.Seq,now.Index,now.Seconds);err!=nil{
   log.Printf("save playback position: %v",err)
  }
 }
 // The first playing status or a track/state transition is sent immediately.
 switched:=prev==nil||prev.Seq!=now.Seq||prev.Index!=now.Index
 stateChanged:=prev!=nil&&prev.State!=now.State
 shouldReport:=switched||stateChanged||now.State=="stopped"||
  time.Since(a.syncer.lastReport)>=10*time.Second
 if !shouldReport{return}
 if prev!=nil&&(prev.Seq!=now.Seq||prev.Index!=now.Index)&&
  prev.State!="stopped"&&a.syncer.previousTrack!=""{
  a.reportTrack(ctx,a.syncer.previousTrack,"stopped",prev.Seconds,prev.Length,true,s.Server)
 }
 track:=s.Tracks[now.Index]
 if a.reportTrack(ctx,track.ID,now.State,now.Seconds,chooseLength(now.Length,track.Duration/1000),false,s.Server) {
  a.syncer.lastReport=time.Now()
 }
 a.syncer.previous=&now
 a.syncer.previousTrack=track.ID
}
func chooseLength(actual,meta int64)int64{
 if actual>0{return actual}
 if meta>0{return meta}
 return 0
}
func (a *app) reportTrack(ctx context.Context,id,state string,sec,length int64,continuing bool,server string)bool{
 if !numericID(id){return false}
 // A saved queue from another Plex server must never alter this server's
 // playback history even when rating keys happen to be the same numbers.
 if server==""||a.client.State.ServerName!=server||
   a.client.State.ServerURL==""||a.client.State.ServerToken==""{
  return false
 }
 reqCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
 defer cancel()
 if err:=a.client.ReportTimeline(reqCtx,id,state,sec,length,continuing);err!=nil{
  log.Printf("Plex playback timeline failed for track %s: %v",id,err)
  return false
 }
 return true
}
func parseFavorite(line string)(kind,id string,value bool,err error){
 fields:=strings.Split(line,"\t")
 if len(fields)!=3{return "","",false,errors.New("invalid favorite event")}
 if fields[1]!="0"&&fields[1]!="1"{return "","",false,errors.New("invalid favorite state")}
 if fields[0]!="item"&&fields[0]!="path"{return "","",false,errors.New("invalid favorite kind")}
 if fields[0]=="item"&&!numericID(fields[2]){return "","",false,errors.New("invalid favorite track")}
 if fields[0]=="path"&&(len(fields[2])>300||!playback.ValidLocal(fields[2])){return "","",false,errors.New("invalid favorite file")}
 return fields[0],fields[2],fields[1]=="1",nil
}
func (a *app) favoriteTrackID(kind,key string)string{
 if kind=="item"{return key}
 if a.offline==nil{return ""}
 for _,col:=range a.offline.List(){
  if col.Server!=a.client.State.ServerName{continue}
  for _,tr:=range col.Tracks{
   if a.offline.MediaPath(tr.File)==key{return tr.ID}
  }
 }
 return ""
}
func (a *app) flushFavorites(ctx context.Context){
 if a.syncer==nil||a.client.State.ServerURL==""||a.client.State.ServerToken==""{return}
 path:=filepath.Join(a.syncer.root,favoritesPath)
 taking:=path+".taking"
 if _,err:=os.Stat(taking);os.IsNotExist(err){
  if err:=os.Rename(path,taking);err!=nil {return}
 }
 data,err:=os.ReadFile(taking)
 if err!=nil||len(data)>128<<10{return}
 // A cut final line cannot be committed and is dropped from the rotated
 // file; it was never acknowledged by Q2 Pod's firmware.
 end:=strings.LastIndexByte(string(data),'\n')
 if end<0 {return}
 entries:=strings.Split(string(data[:end]),"\n")
 for i,line:=range entries{
  if line==""{continue}
  kind,key,fav,parseErr:=parseFavorite(line)
  if parseErr!=nil {
   log.Printf("ignoring malformed Q2 favorite event")
   continue
  }
  id:=a.favoriteTrackID(kind,key)
  if id==""{continue}
  c,cancel:=context.WithTimeout(ctx,5*time.Second)
  err:=a.client.SetFavorite(c,id,fav)
  cancel()
  if err!=nil {
   // Keep the remaining events for retry after the Plex server reconnects.
   remaining:=strings.Join(entries[i:],"\n")+"\n"
   if writeErr:=atomic(taking,[]byte(remaining));writeErr!=nil{
    log.Printf("could not retain unsent favorite events: %v",writeErr)
   }
   return
  }
 }
 _=os.Remove(taking)
}
