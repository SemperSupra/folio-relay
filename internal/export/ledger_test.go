package export

import (
	"errors"
	"path/filepath"
	"testing"
)

func testIntent(p Plan, a Artifact) Intent {
	return Intent{ActionID:"requested-export-0001",JobID:"job-"+a.SHA256,Plan:p,Artifact:a}
}

func TestDurableExportEffectAndNoImplicitReplay(t *testing.T) {
	w,p,a,_,_,_:=fixture(t)
	file:=filepath.Join(t.TempDir(),"ledger","journal.frj")
	l,err:=OpenLedger(file)
	if err!=nil {t.Fatal(err)}
	intent:=testIntent(p,a)
	first,err:=l.Enqueue(intent)
	if err!=nil || first.State!="pending" {t.Fatalf("initial %#v %v",first,err)}
	again,err:=l.Enqueue(intent)
	if err!=nil || again!=first {t.Fatalf("replay %#v %#v %v",first,again,err)}
	intent.Plan.Filename="different.pdf"
	intent.Plan.RelativePath="Prints/2026/different.pdf"
	if _,err:=l.Enqueue(intent);err==nil {t.Fatal("same action changed semantic payload")}
	if _,err:=l.Begin(first.ID);err!=nil {t.Fatal(err)}
	if _,err:=w.Publish(p,a);err!=nil {t.Fatal(err)}
	if err:=l.Close();err!=nil {t.Fatal(err)}
	restarted,err:=OpenLedger(file)
	if err!=nil {t.Fatal(err)}
	defer restarted.Close()
	unresolved,err:=restarted.Get(first.ID)
	if err!=nil || unresolved.State!="reconcile_required" {t.Fatalf("must not assume success: %#v %v",unresolved,err)}
	if _,err:=restarted.Begin(first.ID);!errors.Is(err,ErrReconcile) {t.Fatalf("implicit retry: %v",err)}
	resolved,err:=restarted.ReconcileAfterRestart(first.ID,w)
	if err!=nil || resolved.State!="published" {t.Fatalf("reconcile %#v %v",resolved,err)}
	if _,err:=restarted.Begin(first.ID);!errors.Is(err,ErrReconcile) {t.Fatalf("reused effect: %v",err)}
}

func TestAbsentAfterCrashRequiresNewAction(t *testing.T) {
	w,p,a,_,_,_:=fixture(t)
	file:=filepath.Join(t.TempDir(),"ledger.frj")
	l,err:=OpenLedger(file)
	if err!=nil {t.Fatal(err)}
	intent:=testIntent(p,a)
	e,err:=l.Enqueue(intent)
	if err!=nil {t.Fatal(err)}
	if _,err:=l.Begin(e.ID);err!=nil {t.Fatal(err)}
	if err:=l.Close();err!=nil {t.Fatal(err)}
	again,err:=OpenLedger(file)
	if err!=nil {t.Fatal(err)}
	defer again.Close()
	res,err:=again.ReconcileAfterRestart(e.ID,w)
	if err!=nil || res.State!="reconcile_required" {t.Fatalf("missing export %#v %v",res,err)}
	if _,err:=again.Begin(e.ID);err==nil {t.Fatal("replayed missing operation")}
	intent.ActionID="requested-export-0002"
	newEffect,err:=again.Enqueue(intent)
	if err!=nil || newEffect.ID==e.ID {t.Fatalf("new user action %#v %v",newEffect,err)}
	if _,err:=again.Begin(newEffect.ID);err!=nil {t.Fatal(err)}
	if _,err:=w.Publish(p,a);err!=nil {t.Fatal(err)}
	final,err:=again.Complete(newEffect.ID,true)
	if err!=nil || final.State!="published" {t.Fatalf("finish %#v %v",final,err)}
}

func TestInvalidEffectAndDoubleCompletion(t *testing.T) {
	w,p,a,_,_,_:=fixture(t)
	_ = w
	l,err:=OpenLedger(filepath.Join(t.TempDir(),"ledger.frj"))
	if err!=nil {t.Fatal(err)}
	defer l.Close()
	req:=testIntent(p,a)
	req.ActionID="../escaped"
	if _,err:=l.Enqueue(req);err==nil {t.Fatal("accepted forged action id")}
	req=testIntent(p,a)
	e,err:=l.Enqueue(req)
	if err!=nil {t.Fatal(err)}
	if _,err:=l.Complete(e.ID,true);err==nil {t.Fatal("completed pending effect")}
	if _,err:=l.Begin(e.ID);err!=nil {t.Fatal(err)}
	if _,err:=l.Complete(e.ID,false);err!=nil {t.Fatal(err)}
	if _,err:=l.Complete(e.ID,true);err==nil {t.Fatal("double completion")}
}
