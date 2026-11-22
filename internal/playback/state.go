package playback

import (
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "sync"
 "time"
)

const maxQueueBytes = 256 << 10
const MaxTracks = 250

type Track struct {
 ID string `json:"id"`
 Title string `json:"title"`
 Artist string `json:"artist"`
 Album string `json:"album"`
 Duration int64 `json:"duration"`
 Local string `json:"local,omitempty"`
 Favorite bool `json:"favorite,omitempty"`
}
type Queue struct {
 Version int `json:"version"`
 Server string `json:"server,omitempty"`
 Tracks []Track `json:"tracks"`
 Index int `json:"index"`
 Seconds int64 `json:"seconds"`
 Seq string `json:"seq"`
 Updated int64 `json:"updated"`
}
type Now struct {
 Seq string
 Index int
 State string
 Seconds int64
 Length int64
}
func validID(id string) bool {
 if id==""||len(id)>40{return false}
 for _,c:=range id{if c<'0'||c>'9'{return false}}
 return true
}
func ValidLocal(path string) bool {
 if path==""{return true}
 // A queue can only reference the app's own SD-card music files.
 prefix:="/mnt/mmc/PlexMusic/music/"
 if !strings.HasPrefix(path,prefix)||filepath.Clean(path)!=path{return false}
 name:=strings.TrimPrefix(path,prefix)
 if strings.ContainsAny(name,"/\	
")||len(name)>80{return false}
 dash:=strings.IndexByte(name,'-')
 if dash!=8{return false}
 for _,ch:=range name[:dash]{
  if !(ch>='0'&&ch<='9'||ch>='a'&&ch<='f'){return false}
 }
 dot:=strings.LastIndexByte(name,'.')
 return dot>dash+1&&validID(name[dash+1:dot])
}
func (q Queue) validate() error {
 if q.Version!=1||len(q.Tracks)>MaxTracks{return errors.New("unsupported saved queue")}
 if len(q.Tracks)==0{return nil}
 if q.Index<0||q.Index>=len(q.Tracks)||q.Seconds<0||len(q.Server)>200{return errors.New("invalid resume position")}
 if len(q.Seq)>23{return errors.New("invalid queue sequence")}
 for _,t:=range q.Tracks {
  if !validID(t.ID)||!ValidLocal(t.Local)||t.Duration<0||t.Duration>86400000{
   return errors.New("invalid saved track")
  }
  if len(t.Title)>512||len(t.Artist)>512||len(t.Album)>512{return errors.New("saved metadata too long")}
 }
 return nil
}
type Store struct {
 mu sync.Mutex
 path string
 queue Queue
}
func Open(path string)(*Store,error){
 s:=&Store{path:path,queue:Queue{Version:1}}
 data,err:=os.ReadFile(path)
 if errors.Is(err,os.ErrNotExist){return s,nil}
 if err!=nil{return nil,err}
 if len(data)>maxQueueBytes{return nil,errors.New("saved queue too large")}
 if err=json.Unmarshal(data,&s.queue);err!=nil{return nil,fmt.Errorf("saved queue: %w",err)}
 if err=s.queue.validate();err!=nil{return nil,err}
 return s,nil
}
func (s *Store) Snapshot()Queue{
 s.mu.Lock();defer s.mu.Unlock()
 q:=s.queue
 q.Tracks=append([]Track(nil),q.Tracks...)
 return q
}
func (s *Store) persist(q Queue)error {
 if err:=q.validate();err!=nil{return err}
 data,err:=json.Marshal(q);if err!=nil{return err}
 if len(data)>maxQueueBytes{return errors.New("saved queue too large")}
 if err=os.MkdirAll(filepath.Dir(s.path),0700);err!=nil{return err}
 f,err:=os.OpenFile(s.path+".tmp",os.O_CREATE|os.O_WRONLY|os.O_TRUNC,0600)
 if err!=nil{return err}
 if _,err=f.Write(data);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return os.Rename(s.path+".tmp",s.path)
}
func (s *Store) Replace(q Queue)error {
 s.mu.Lock();defer s.mu.Unlock()
 q.Version=1
 q.Updated=time.Now().Unix()
 if err:=s.persist(q);err!=nil{return err}
 s.queue=q
 return nil
}
func (s *Store) Update(seq string,index int,seconds int64)error{
 s.mu.Lock();defer s.mu.Unlock()
 q:=s.queue
 if len(q.Tracks)==0||q.Seq!=seq||index<0||index>=len(q.Tracks)||seconds<0{
  return nil
 }
 if seconds>q.Tracks[index].Duration/1000 && q.Tracks[index].Duration>0{
  seconds=q.Tracks[index].Duration/1000
 }
 if q.Index==index&&q.Seconds==seconds{return nil}
 q.Index=index;q.Seconds=seconds;q.Updated=time.Now().Unix()
 if err:=s.persist(q);err!=nil{return err}
 s.queue=q
 return nil
}
func (s *Store) Clear()error{
 return s.Replace(Queue{Version:1})
}
func ParseNow(raw []byte)(Now,error){
 if len(raw)==0||len(raw)>256{return Now{},errors.New("invalid playback status size")}
 fields:=strings.Split(strings.TrimSpace(string(raw)),"	")
 if len(fields)!=5{return Now{},errors.New("invalid playback status fields")}
 seq:=fields[0]
 if len(seq)==0||len(seq)>23{return Now{},errors.New("invalid playback sequence")}
 for _,c:=range seq{if c<'0'||c>'9'{return Now{},errors.New("invalid playback sequence")}}
 index,err:=strconv.Atoi(fields[1]);if err!=nil||index<0||index>=MaxTracks{return Now{},errors.New("invalid playback index")}
 state:=fields[2]
 if state!="playing"&&state!="paused"&&state!="stopped"{return Now{},errors.New("invalid playback state")}
 sec,err:=strconv.ParseInt(fields[3],10,64);if err!=nil||sec<0||sec>86400{return Now{},errors.New("invalid playback position")}
 length,err:=strconv.ParseInt(fields[4],10,64);if err!=nil||length<0||length>86400{return Now{},errors.New("invalid playback length")}
 return Now{Seq:seq,Index:index,State:state,Seconds:sec,Length:length},nil
}
