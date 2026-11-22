package offline

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func addTrack(t *testing.T,s *Store,server,id string)Track{
 t.Helper()
 file:=MediaFile(server,id,".flac")
 if !ValidFile(file){t.Fatalf("invalid generated filename %q",file)}
 bytes:=[]byte("fLaC0123456789")
 if err:=os.WriteFile(s.MediaPath(file),bytes,0600);err!=nil{t.Fatal(err)}
 return Track{ID:id,File:file,Title:"Test",Artist:"Band",Album:"Album",Size:int64(len(bytes))}
}
func TestFilenameValidation(t *testing.T){
 good:=MediaFile("Plex NAS","12345",".flac")
 if !ValidFile(good){t.Fatalf("rejected own filename %q",good)}
 for _,bad:=range []string{"../audio.flac","/etc/passwd","../../aaa","deadbeef-abc.flac","deadbeef-123.mp4","a.flac","deadbeef-123.flac/"}{
  if ValidFile(bad){t.Errorf("accepted filename %q",bad)}
 }
}
func TestPartialCollectionPersistsAndCompletes(t *testing.T){
 root:=filepath.Join(t.TempDir(),"PlexMusic")
 s,err:=Load(root);if err!=nil{t.Fatal(err)}
 server:="Plex NAS"
 key:=CollectionKey(server,"album","77")
 if err=s.Begin(server,"album","77","My Album");err!=nil{t.Fatal(err)}
 tr:=addTrack(t,s,server,"12")
 if err=s.Append(key,tr);err!=nil{t.Fatal(err)}
 reopened,err:=Load(root);if err!=nil{t.Fatal(err)}
 before,ok:=reopened.Get(key)
 if !ok||before.Complete||len(before.Tracks)!=1||!reopened.Exists(before.Tracks[0]){t.Fatalf("partial collection %+v",before)}
 if err=reopened.Finish(key,[]Track{tr,tr});err!=nil{t.Fatal(err)}
 reopened,err=Load(root);if err!=nil{t.Fatal(err)}
 col,ok:=reopened.Get(key)
 if !ok||!col.Complete||len(col.Tracks)!=2{t.Fatalf("playlist order/duplicates lost %+v",col)}
 if err=reopened.Remove(key);err!=nil{t.Fatal(err)}
 if _,err=os.Stat(reopened.MediaPath(tr.File));!os.IsNotExist(err){t.Errorf("orphan not removed: %v",err)}
}
func TestSharedMediaPreservedWhenRemovingOneCollection(t *testing.T){
 s,err:=Load(filepath.Join(t.TempDir(),"PlexMusic"));if err!=nil{t.Fatal(err)}
 server:="My Server"
 tr:=addTrack(t,s,server,"51")
 album:=CollectionKey(server,"album","22")
 playlist:=CollectionKey(server,"playlist","33")
 for _,k:=range []struct{kind,id,key string}{{"album","22",album},{"playlist","33",playlist}}{
  if err=s.Begin(server,k.kind,k.id,k.kind);err!=nil{t.Fatal(err)}
  if err=s.Append(k.key,tr);err!=nil{t.Fatal(err)}
  if err=s.Finish(k.key,[]Track{tr});err!=nil{t.Fatal(err)}
 }
 if err=s.Remove(album);err!=nil{t.Fatal(err)}
 if _,err=os.Stat(s.MediaPath(tr.File));err!=nil{t.Fatalf("removed shared track: %v",err)}
 if err=s.Remove(playlist);err!=nil{t.Fatal(err)}
 if _,err=os.Stat(s.MediaPath(tr.File));!os.IsNotExist(err){t.Errorf("last shared track not removed: %v",err)}
}
func TestCorruptManifestIsNotOverwritten(t *testing.T){
 root:=filepath.Join(t.TempDir(),"PlexMusic")
 if err:=os.MkdirAll(root,0700);err!=nil{t.Fatal(err)}
 content:=[]byte("{corrupt")
 if err:=os.WriteFile(filepath.Join(root,"index.json"),content,0600);err!=nil{t.Fatal(err)}
 if _,err:=Load(root);err==nil{t.Fatal("ignored invalid manifest")}
 b,err:=os.ReadFile(filepath.Join(root,"index.json"));if err!=nil{t.Fatal(err)}
 if string(b)!="{corrupt"{t.Fatal("clobbered manifest")}
}
func TestTrackIsOnlyAvailableAfterCompleteFile(t *testing.T){
 s,err:=Load(filepath.Join(t.TempDir(),"PlexMusic"));if err!=nil{t.Fatal(err)}
 f:=MediaFile("NAS","11",".mp3")
 path:=s.MediaPath(f)
 if err:=os.WriteFile(path+".part",[]byte("half"),0600);err!=nil{t.Fatal(err)}
 if s.Exists(Track{ID:"11",File:f,Size:4}){t.Fatal("partial file treated as music")}
 if err:=os.WriteFile(path,[]byte("done"),0600);err!=nil{t.Fatal(err)}
 if !s.Exists(Track{ID:"11",File:f,Size:4}){t.Fatal("complete track missing")}
 if strings.Contains(path,".plex"){t.Fatal("music folder should be visible under card root")}
}
