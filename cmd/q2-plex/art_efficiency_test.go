package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)
func TestArtworkCachePrunesEntries(t *testing.T){
	dir:=t.TempDir()
	a:=&artworkCache{dir:dir}
	for i:=0;i<106;i++{
		path:=filepath.Join(dir,fmt.Sprintf("%08x.jpg",i))
		if err:=os.WriteFile(path,[]byte("jpeg"),0600);err!=nil{t.Fatal(err)}
		when:=time.Now().Add(-time.Duration(107-i)*time.Minute)
		_ =os.Chtimes(path,when,when)
	}
	a.prune()
	entries,err:=os.ReadDir(dir);if err!=nil{t.Fatal(err)}
	if len(entries)!=artEntries{t.Fatalf("cache entries=%d want %d",len(entries),artEntries)}
}
