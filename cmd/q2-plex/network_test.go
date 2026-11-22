package main

import (
	"errors"
	"testing"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
)
func TestReconnectBackoffBounded(t *testing.T){
	expected:=[]time.Duration{30,60,120,240,300,300}
	for i,want:=range expected {
		if got:=reconnectDelay(i+1);got!=want*time.Second{
			t.Fatalf("failure %d delay %v want %v",i+1,got,want)
		}
	}
	s:=connectionStatus{}
	now:=time.Now()
	s.record(now,&plex.HTTPStatusError{Code:503})
	if s.online||s.failures!=1||!s.nextAttempt.Equal(now.Add(30*time.Second)){t.Fatalf("bad failure state %+v",s)}
	s.record(now,nil)
	if !s.online||s.failures!=0||!s.nextAttempt.IsZero(){t.Fatalf("bad recovery %+v",s)}
}
func TestRootStreamingFirstAndConnectionMenu(t *testing.T){
	a:=&app{
		client:&plex.Client{State:plex.State{Token:"signed",ServerName:"Plex NAS",LibraryID:"12"}},
		connection:connectionStatus{online:true,started:true},
	}
	root:=a.root()
	if len(root.Rows)<8||root.Rows[0].URI!="albums"{t.Fatalf("streaming should be first: %+v",root.Rows)}
	var offline,connection bool
	for _,r:=range root.Rows{
		offline=offline||r.URI=="offline"
		connection=connection||r.URI=="connection"
	}
	if !offline||!connection{t.Fatal("missing offline/connection menus")}
	page:=a.connectionPage()
	if page.Rows[0].Title!="Connected: Plex NAS"{t.Fatalf("wrong connection status %+v",page.Rows)}
}
func TestNetworkFailureDetection(t *testing.T){
	a:=&app{connection:connectionStatus{started:true,online:true}}
	a.notePlexFailure(errors.New("invalid metadata"))
	if !a.connection.online{t.Fatal("non-network error disconnected client")}
	a.notePlexFailure(&plex.HTTPStatusError{Code:503})
	if a.connection.online||a.connection.failures!=1{t.Fatalf("missing network failure %+v",a.connection)}
}
