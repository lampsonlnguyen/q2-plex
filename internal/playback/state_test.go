package playback

import (
 "os"
 "path/filepath"
 "testing"
)
func TestParseNow(t *testing.T){
 n,err:=ParseNow([]byte("123456789\t1\tpaused\t53\t223\n"))
 if err!=nil||n.Index!=1||n.Seconds!=53||n.State!="paused"{t.Fatalf("%+v %v",n,err)}
 for _,bad:=range []string{"hello\t1\tplaying\t12\t50","100\t-1\tplaying\t0\t60","100\t0\tbuffering\t0\t60","100\t0\tplaying\t-10\t60","100\t0\tplaying\t0\t999999","100\t0\tplaying\t0"}{
  if _,err:=ParseNow([]byte(bad));err==nil{t.Errorf("accepted %q",bad)}
 }
}
func TestSaveAndRestoreTrackAndPosition(t *testing.T){
 dir:=t.TempDir();path:=filepath.Join(dir,"last-queue.json")
 s,err:=Open(path);if err!=nil{t.Fatal(err)}
 q:=Queue{Version:1,Server:"Plex NAS",Seq:"12345",Index:1,Tracks:[]Track{
  {ID:"100",Title:"First",Artist:"Artist",Album:"Album",Duration:180000},
  {ID:"101",Title:"Second",Artist:"Artist",Album:"Album",Duration:220000},
 }}
 if err=s.Replace(q);err!=nil{t.Fatal(err)}
 if err=s.Update("12345",1,73);err!=nil{t.Fatal(err)}
 if err=s.Update("different",0,90);err!=nil{t.Fatal(err)}
 restored,err:=Open(path);if err!=nil{t.Fatal(err)}
 out:=restored.Snapshot()
 if out.Index!=1||out.Seconds!=73||len(out.Tracks)!=2||out.Tracks[1].ID!="101"{t.Fatalf("wrong restored %+v",out)}
 stat,err:=os.Stat(path);if err!=nil{t.Fatal(err)}
 if stat.Mode().Perm()&0077!=0{t.Fatalf("queue file permissions %v",stat.Mode())}
 if err=restored.Clear();err!=nil{t.Fatal(err)}
 if len(restored.Snapshot().Tracks)!=0{t.Fatal("queue not cleared")}
}
func TestRejectUnsafeSavedLocalPaths(t *testing.T){
 for _,p:=range []string{"/etc/passwd","/mnt/mmc/PlexMusic/music/../../.plex/auth.json","/mnt/mmc/PlexMusic/music/foo.mp3","/mnt/mmc/PlexMusic/music/deadbeef-bad.mp3","/tmp/PlexMusic/music/deadbeef-123.mp3"}{
  if ValidLocal(p){t.Errorf("accepted unsafe path %q",p)}
 }
 if !ValidLocal("/mnt/mmc/PlexMusic/music/deadbeef-123.flac"){t.Fatal("valid local music path rejected")}
}
