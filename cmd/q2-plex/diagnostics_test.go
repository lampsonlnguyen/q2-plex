package main

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

func TestDiagnosticCSVNoPrivateMetadata(t *testing.T){
	data,err:=encodeDiagnostics([]diagnosticSample{{At:time.Date(2026,11,22,12,0,0,0,time.UTC),UptimeSeconds:60,CPUSeconds:2.5,CPUPct:4.17,HeapKiB:5120,HeapSysKiB:8096,MaxRSSKiB:12000,Goroutines:9,BatteryPct:-1}})
	if err!=nil{t.Fatal(err)}
	rows,err:=csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err!=nil{t.Fatal(err)}
	if len(rows)!=2||len(rows[0])!=9||len(rows[1])!=9{t.Fatalf("bad CSV: %q",data)}
	if rows[1][8]!=""{t.Fatalf("missing battery should be blank: %q",rows[1][8])}
	for _,private:=range []string{"X-Plex-Token","auth.json","/mnt/mmc/","Bearer"}{
		if strings.Contains(string(data),private){t.Fatalf("private field in CSV: %q",private)}
	}
}
func TestDiagnosticCSVEmpty(t *testing.T){
	if _,err:=encodeDiagnostics(nil);err==nil{t.Fatal("empty samples accepted")}
}
func TestBatteryCapacity(t *testing.T){
	dir:=t.TempDir()
	mk:=func(name,kind,capacity string){
		p:=filepath.Join(dir,name)
		if err:=os.MkdirAll(p,0700);err!=nil{t.Fatal(err)}
		if err:=os.WriteFile(filepath.Join(p,"type"),[]byte(kind),0600);err!=nil{t.Fatal(err)}
		if err:=os.WriteFile(filepath.Join(p,"capacity"),[]byte(capacity),0600);err!=nil{t.Fatal(err)}
	}
	mk("USB","USB","100")
	mk("BAT0","Battery","73\n")
	if got:=batteryCapacity(dir);got!=73{t.Fatalf("battery %d",got)}
	if err:=os.Symlink(filepath.Join(dir,"BAT0"),filepath.Join(dir,"BatteryLink"));err!=nil{t.Fatal(err)}
	// The real device's power_supply class normally has symlinks.
	if got:=batteryCapacity(dir);got!=73{t.Fatalf("symlinked battery %d",got)}
	if got:=batteryCapacity(filepath.Join(dir,"not-found"));got!=-1{t.Fatalf("missing battery %d",got)}
}
func TestDiagnosticRingBounded(t *testing.T){
	d:=&diagnostics{}
	d.active=true
	d.start=time.Now().Add(-time.Minute)
	for i:=0;i<diagnosticSampleLimit+12;i++{d.take()}
	_,samples:=d.snapshot()
	if len(samples)!=diagnosticSampleLimit{t.Fatalf("samples %d",len(samples))}
	d.stopSampling()
}
func TestDiagnosticsExplicitExport(t *testing.T){
	dir:=t.TempDir()
	a:=&app{diag:&diagnostics{},diagDir:dir}
	if page:=a.diagnosticPage();len(page.Rows)==0{t.Fatal("missing diagnostics page")}
	if got,_:=a.handleDiagnostics(q2spot.Request{Op:"diagexport"});got.Status!="error"{t.Fatalf("exported empty diagnostics %+v",got)}
	if _,err:=os.Stat(filepath.Join(dir,"diagnostics.csv"));!os.IsNotExist(err){t.Fatalf("unexpected SD write: %v",err)}
	a.diag.mu.Lock()
	a.diag.samples=append(a.diag.samples,diagnosticSample{At:time.Now(),CPUSeconds:1,BatteryPct:-1})
	a.diag.mu.Unlock()
	reply,ok:=a.handleDiagnostics(q2spot.Request{Op:"diagexport"})
	if !ok||reply.Status=="error"{t.Fatalf("export failed: %+v",reply)}
	b,err:=os.ReadFile(filepath.Join(dir,"diagnostics.csv"));if err!=nil{t.Fatal(err)}
	rows,err:=csv.NewReader(strings.NewReader(string(b))).ReadAll()
	if err!=nil||len(rows)!=2{t.Fatalf("bad diagnostics file %v %q",err,b)}
}
func TestDiagnosticsStartStop(t *testing.T){
	d:=&diagnostics{}
	if !d.startSampling(context.Background()){t.Fatal("first start refused")}
	if d.startSampling(context.Background()){t.Fatal("duplicate start")}
	if !d.stopSampling(){t.Fatal("failed stop")}
	if d.stopSampling(){t.Fatal("stopped twice")}
}
func TestDiagnosticFormat(t *testing.T){
	s:=diagnosticSample{CPUPct:7.5,MaxRSSKiB:4096,HeapKiB:2048,BatteryPct:66}
	out:=formatSample(s)
	for _,part:=range []string{"7.5%","4096 KiB","2048 KiB","66%"}{
		if !strings.Contains(out,part){t.Fatalf("missing %s in %s",part,out)}
	}
	_ = strconv.Itoa(len(out))
}
