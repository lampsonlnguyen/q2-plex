package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
	"github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

// Reconnect only when an actual connection has failed. No continuous LAN
// scanning or Plex cloud requests while everything is healthy.
type connectionStatus struct {
	started bool
	online bool
	failures int
	nextAttempt time.Time
}
func reconnectDelay(failures int)time.Duration {
	if failures<=0{return 0}
	seconds:=30
	for i:=1;i<failures&&seconds<300;i++{seconds*=2}
	if seconds>300{seconds=300}
	return time.Duration(seconds)*time.Second
}
func (s *connectionStatus) record(at time.Time,err error){
	if err==nil{
		s.online=true;s.failures=0;s.nextAttempt=time.Time{}
		return
	}
	s.online=false
	s.failures++
	s.nextAttempt=at.Add(reconnectDelay(s.failures))
}
func connectionLabel(server string,online,busy bool)string{
	if busy{return "Connecting"}
	if online{return "Connected: "+server}
	if server!=""{return "Reconnecting"}
	return "Set up Plex"
}
func (a *app) connectionPage()q2spot.Reply{
	a.mu.Lock()
	server:=a.client.State.ServerName
	online,busy:=a.connection.online,a.busy
	failures:=a.connection.failures
	next:=a.connection.nextAttempt
	message:=a.lastError
	a.mu.Unlock()
	status:=connectionLabel(server,online,busy)
	rows:=[]q2spot.Row{text(status)}
	if !online&&!busy&&failures>0{
		remaining:=time.Until(next)
		if remaining>0{rows=append(rows,text(fmt.Sprintf("Auto-retry in %ds",int(remaining.Seconds())+1)))}
	}
	rows=append(rows,
		act("reconnect","Reconnect Now",""),
		list("manual","Server Address","Set manually",""),
		act("logout","Sign Out",""),
	)
	if message!=""&&!online{rows=append(rows,text("Error: "+message))}
	return q2spot.Reply{Rows:rows}
}
func (a *app) notePlexFailure(err error){
	if !plex.Retryable(err){return}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.connection.started{return}
	a.connection.record(time.Now(),err)
	a.lastError=err.Error()
}
func (a *app) reconnectWorker(ctx context.Context){
	timer:=time.NewTicker(15*time.Second)
	defer timer.Stop()
	for {
		select{
		case <-ctx.Done():return
		case <-timer.C:
			a.mu.Lock()
			ready:=a.connection.started&&!a.connection.online&&!a.busy&&
				a.client.State.Token!=""&& !a.download.Active&&
				!time.Now().Before(a.connection.nextAttempt)
			a.mu.Unlock()
			if ready{a.launchConnection("")}
		}
	}
}
