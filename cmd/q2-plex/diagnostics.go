package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

const diagnosticInterval = 60 * time.Second
const diagnosticSampleLimit = 120

// These are process-level measurements, not system-wide battery benchmarks.
// Collection is disabled by default. Nothing is written to the SD card until
// the user explicitly exports an anonymized CSV.
type diagnosticSample struct {
	At time.Time
	UptimeSeconds int64
	CPUSeconds float64
	CPUPct float64
	HeapKiB uint64
	HeapSysKiB uint64
	MaxRSSKiB int64
	Goroutines int
	BatteryPct int // -1: no readable battery capacity
}
type diagnostics struct {
	mu sync.Mutex
	active bool
	start time.Time
	cancel context.CancelFunc
	samples []diagnosticSample
}
func cpuSeconds() (float64,int64,error) {
	var usage syscall.Rusage
	if err:=syscall.Getrusage(syscall.RUSAGE_SELF,&usage);err!=nil{return 0,0,err}
	// Linux maximum resident set size is reported in KiB.
	user:=float64(usage.Utime.Sec)+float64(usage.Utime.Usec)/1e6
	system:=float64(usage.Stime.Sec)+float64(usage.Stime.Usec)/1e6
	return user+system,usage.Maxrss,nil
}
func batteryCapacity(root string) int {
	entries,err:=os.ReadDir(root)
	if err!=nil{return -1}
	for _,entry:=range entries{
		base:=filepath.Join(root,entry.Name())
		// sysfs class entries are often symlinks to actual power supplies.
		info,err:=os.Stat(base)
		if err!=nil||!info.IsDir(){continue}
		kind,err:=os.ReadFile(filepath.Join(base,"type"))
		if err!=nil||!strings.EqualFold(strings.TrimSpace(string(kind)),"Battery"){continue}
		data,err:=os.ReadFile(filepath.Join(base,"capacity"))
		if err!=nil{continue}
		n,err:=strconv.Atoi(strings.TrimSpace(string(data)))
		if err==nil&&n>=0&&n<=100{return n}
	}
	return -1
}
func takeDiagnosticSample(start time.Time,previous *diagnosticSample) (diagnosticSample,error) {
	now:=time.Now()
	cpu,rss,err:=cpuSeconds()
	if err!=nil{return diagnosticSample{},err}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	sample:=diagnosticSample{
		At:now,UptimeSeconds:int64(now.Sub(start).Seconds()),CPUSeconds:cpu,
		HeapKiB:mem.HeapAlloc/1024,HeapSysKiB:mem.HeapSys/1024,
		MaxRSSKiB:rss,Goroutines:runtime.NumGoroutine(),
		BatteryPct:batteryCapacity("/sys/class/power_supply"),
	}
	if previous!=nil{
		elapsed:=now.Sub(previous.At).Seconds()
		if elapsed>0{
			// One core fully occupied equals 100%. May exceed 100% with threads.
			pct:=100*(cpu-previous.CPUSeconds)/elapsed
			if pct>=0{sample.CPUPct=pct}
		}
	}
	return sample,nil
}
func (d *diagnostics) startSampling(parent context.Context) bool {
	d.mu.Lock()
	if d.active{d.mu.Unlock();return false}
	ctx,cancel:=context.WithCancel(parent)
	d.active=true
	d.start=time.Now()
	d.samples=nil
	d.cancel=cancel
	d.mu.Unlock()
	go d.run(ctx)
	return true
}
func (d *diagnostics) run(ctx context.Context){
	ticker:=time.NewTicker(diagnosticInterval)
	defer ticker.Stop()
	for {
		d.take()
		select {case <-ctx.Done():return;case <-ticker.C:}
	}
}
func (d *diagnostics) take(){
	d.mu.Lock()
	if !d.active{d.mu.Unlock();return}
	start:=d.start
	var previous *diagnosticSample
	if n:=len(d.samples);n>0 {prior:=d.samples[n-1];previous=&prior}
	d.mu.Unlock()
	s,err:=takeDiagnosticSample(start,previous)
	if err!=nil{return}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active||!d.start.Equal(start){return}
	if len(d.samples)==diagnosticSampleLimit{copy(d.samples,d.samples[1:]);d.samples=d.samples[:diagnosticSampleLimit-1]}
	d.samples=append(d.samples,s)
}
func (d *diagnostics) stopSampling() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.active{return false}
	d.active=false
	if d.cancel!=nil{d.cancel();d.cancel=nil}
	return true
}
func (d *diagnostics) snapshot() (bool,[]diagnosticSample){
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active,append([]diagnosticSample(nil),d.samples...)
}
func formatSample(s diagnosticSample) string {
	battery:="unavailable"
	if s.BatteryPct>=0{battery=fmt.Sprintf("%d%%",s.BatteryPct)}
	return fmt.Sprintf("CPU %.1f%%, RSS peak %d KiB, heap %d KiB, battery %s",s.CPUPct,s.MaxRSSKiB,s.HeapKiB,battery)
}
func encodeDiagnostics(samples []diagnosticSample)([]byte,error){
	if len(samples)==0{return nil,errors.New("no diagnostic samples to export")}
	var b strings.Builder
	writer:=csv.NewWriter(&b)
	if err:=writer.Write([]string{"time_utc","uptime_seconds","process_cpu_seconds","process_cpu_percent","heap_kib","heap_sys_kib","max_rss_kib","goroutines","battery_percent"});err!=nil{return nil,err}
	for _,s:=range samples{
		battery:=""
		if s.BatteryPct>=0{battery=strconv.Itoa(s.BatteryPct)}
		err:=writer.Write([]string{
			s.At.UTC().Format(time.RFC3339),strconv.FormatInt(s.UptimeSeconds,10),
			strconv.FormatFloat(s.CPUSeconds,'f',3,64),
			strconv.FormatFloat(s.CPUPct,'f',2,64),
			strconv.FormatUint(s.HeapKiB,10),strconv.FormatUint(s.HeapSysKiB,10),
			strconv.FormatInt(s.MaxRSSKiB,10),strconv.Itoa(s.Goroutines),battery,
		})
		if err!=nil{return nil,err}
	}
	writer.Flush()
	if err:=writer.Error();err!=nil{return nil,err}
	return []byte(b.String()),nil
}
func (a *app) diagnosticPage()q2spot.Reply{
	if a.diag==nil{return q2spot.Reply{Rows:[]q2spot.Row{text("Diagnostics unavailable")}}}
	active,samples:=a.diag.snapshot()
	status:="Off (no background sampling)"
	if active{status="Recording every 60 seconds"}
	rows:=[]q2spot.Row{text(status)}
	if active{rows=append(rows,act("diagstop","Stop Recording",""))}else{rows=append(rows,act("diagstart","Start Recording",""))}
	rows=append(rows,text(fmt.Sprintf("Samples: %d (max %d)",len(samples),diagnosticSampleLimit)))
	if n:=len(samples);n>0{
		last:=samples[n-1]
		rows=append(rows,text(formatSample(last)),text(fmt.Sprintf("Session: %ds",last.UptimeSeconds)),act("diagexport","Export CSV to SD",""))
	}
	rows=append(rows,text("CPU and memory are q2-plex process only"),text("Battery percentage is a device snapshot, not a power measurement"))
	return q2spot.Reply{Rows:rows}
}
func (a *app) handleDiagnostics(req q2spot.Request)(q2spot.Reply,bool){
	switch req.Op{
	case "diagnostics":return a.diagnosticPage(),true
	case "diagstart":
		if a.diag==nil{return q2spot.Fail("error","Diagnostics unavailable"),true}
		parent:=a.ctx
		if parent==nil{parent=context.Background()}
		a.diag.startSampling(parent)
		return a.diagnosticPage(),true
	case "diagstop":
		if a.diag!=nil{a.diag.stopSampling()}
		return a.diagnosticPage(),true
	case "diagexport":
		if a.diag==nil{return q2spot.Fail("error","Diagnostics unavailable"),true}
		_,samples:=a.diag.snapshot()
		data,err:=encodeDiagnostics(samples)
		if err!=nil{return q2spot.Fail("error",err.Error()),true}
		if err=atomic(filepath.Join(a.diagDir,"diagnostics.csv"),data);err!=nil{
			return q2spot.Fail("error","Could not write SD diagnostics"),true
		}
		return q2spot.Reply{Message:"Saved .plex/diagnostics.csv"},true
	}
	return q2spot.Reply{},false
}
