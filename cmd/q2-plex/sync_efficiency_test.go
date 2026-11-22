package main

import (
	"testing"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/playback"
)
func TestAdaptivePlaybackPolling(t *testing.T){
	if nextPlaybackPoll(nil)!=30*time.Second{t.Fatal("idle should poll slowly")}
	for _,state:=range []string{"playing","paused"}{
		if nextPlaybackPoll(&playback.Now{State:state})!=5*time.Second{t.Fatalf("%s should poll at 5s",state)}
	}
	if nextPlaybackPoll(&playback.Now{State:"stopped"})!=30*time.Second{t.Fatal("stopped should poll slowly")}
}
func TestPlaybackBookmarkWriteThrottling(t *testing.T){
	old:=playback.Queue{Seconds:30,Index:0}
	prev:=&playback.Now{Seq:"1",Index:0,State:"playing",Seconds:50}
	if shouldSavePosition(old,prev,playback.Now{Seq:"1",Index:0,State:"playing",Seconds:55}){
		t.Fatal("short progress should not write SD")
	}
	if !shouldSavePosition(old,prev,playback.Now{Seq:"1",Index:0,State:"playing",Seconds:61}){
		t.Fatal("30 second progress should save")
	}
	if !shouldSavePosition(old,prev,playback.Now{Seq:"1",Index:1,State:"playing",Seconds:3}){
		t.Fatal("track switch must save immediately")
	}
	if !shouldSavePosition(old,prev,playback.Now{Seq:"1",Index:0,State:"paused",Seconds:45}){
		t.Fatal("pause must save immediately")
	}
	if !shouldSavePosition(old,prev,playback.Now{Seq:"1",Index:0,State:"playing",Seconds:7}){
		t.Fatal("seek backward must save immediately")
	}
}
