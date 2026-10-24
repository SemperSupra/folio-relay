package export

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"runtime"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Worker, Plan, Artifact, []byte, string, string) {
	t.Helper()
	root := t.TempDir()
	canonical := filepath.Join(root,"canonical")
	exports := filepath.Join(root,"exports")
	if err := os.MkdirAll(canonical,0700); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(exports,0700); err != nil { t.Fatal(err) }
	data := []byte("%PDF-1.7\ncontrolled fixture\n")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	src := filepath.Join(canonical,"sha256",digest[:2],digest)
	if err := os.MkdirAll(filepath.Dir(src),0700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(src,data,0600); err != nil { t.Fatal(err) }
	w,err := NewWorker(canonical,exports)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ w.Close() })
	job := Job{JobID:"job-"+strings.Repeat("a",64),AcceptedAt:time.Date(2026,10,24,10,0,0,0,time.UTC),SourceTitle:"Quarterly/Report",MediaType:"application/pdf"}
	p,err := MakePlan(Defaults{Folder:"Prints/2026"},Overrides{},job)
	if err != nil { t.Fatal(err) }
	return w,p,Artifact{SHA256:digest,Bytes:int64(len(data))},data,src,exports
}

func TestPublishVerifiedAndCanonicalUntouched(t *testing.T) {
	w,p,a,data,src,root := fixture(t)
	receipt,err := w.Publish(p,a)
	if err != nil { t.Fatal(err) }
	if receipt.RelativePath != p.RelativePath || receipt.Bytes != int64(len(data)) { t.Fatalf("bad receipt %#v",receipt) }
	got,err := os.ReadFile(filepath.Join(root, filepath.FromSlash(receipt.RelativePath)))
	if err != nil || string(got)!=string(data) { t.Fatalf("export mismatch %q %v",got,err) }
	got,err=os.ReadFile(src)
	if err != nil || string(got)!=string(data) { t.Fatalf("canonical was modified %q %v",got,err) }
	state,err:=w.Inspect(receipt)
	if err != nil || state!="present" { t.Fatalf("bad state %q %v",state,err) }
	if _,err:=w.Publish(p,a); !errors.Is(err,ErrCollision) { t.Fatalf("must not overwrite destination: %v",err) }
}

func TestSMBDeleteAndRenameCanBeReExported(t *testing.T) {
	w,p,a,data,src,root:=fixture(t)
	receipt,err:=w.Publish(p,a)
	if err!=nil { t.Fatal(err) }
	target:=filepath.Join(root,filepath.FromSlash(p.RelativePath))
	renamed:=target+".user-renamed"
	if err:=os.Rename(target,renamed);err!=nil { t.Fatal(err) }
	state,err:=w.Inspect(receipt)
	if err!=nil || state!="missing_or_moved" { t.Fatalf("rename %s %v",state,err) }
	if _,err:=w.Publish(p,a);err!=nil { t.Fatalf("explicit reexport: %v",err) }
	if err:=os.Remove(target);err!=nil { t.Fatal(err) }
	state,err=w.Inspect(receipt)
	if err!=nil || state!="missing_or_moved" { t.Fatalf("delete %s %v",state,err) }
	if _,err:=w.Publish(p,a);err!=nil { t.Fatalf("explicit reexport after delete: %v",err) }
	stored,err:=os.ReadFile(src)
	if err!=nil || string(stored)!=string(data) { t.Fatal("canonical should be unaffected by SMB operations") }
}

func TestAlteredExportIsReportedWithoutReplacement(t *testing.T) {
	w,p,a,_,_,root:=fixture(t)
	receipt,err:=w.Publish(p,a)
	if err!=nil { t.Fatal(err) }
	target:=filepath.Join(root,filepath.FromSlash(p.RelativePath))
	if err:=os.WriteFile(target,[]byte("user edited"),0600);err!=nil { t.Fatal(err) }
	status,err:=w.Inspect(receipt)
	if err!=nil || status!="modified" { t.Fatalf("modified status %q %v",status,err) }
	if _,err:=w.Publish(p,a);!errors.Is(err,ErrCollision) { t.Fatalf("modified export must not be overwritten: %v",err) }
}

func TestModifiedCanonicalRejectsPublication(t *testing.T) {
	w,p,a,_,src,root:=fixture(t)
	if err:=os.WriteFile(src,[]byte("corrupt"),0600);err!=nil { t.Fatal(err) }
	if _,err:=w.Publish(p,a);!errors.Is(err,ErrSource) { t.Fatalf("expected source validation failure: %v",err) }
	if _,err:=os.Stat(filepath.Join(root,filepath.FromSlash(p.RelativePath)));!errors.Is(err,os.ErrNotExist) { t.Fatalf("must not publish invalid bytes: %v",err) }
}

func TestSymlinkEscapeCannotWriteOutsideRoot(t *testing.T) {
	w,p,a,_,_,root:=fixture(t)
	outside:=t.TempDir()
	if err:=os.Symlink(outside,filepath.Join(root,"Prints"));err!=nil {
		t.Skipf("symlinks unavailable: %v",err)
	}
	if _,err:=w.Publish(p,a);err==nil { t.Fatal("symlinked export directory must be rejected") }
	items,err:=os.ReadDir(outside)
	if err!=nil || len(items)!=0 { t.Fatal("wrote outside export root") }
}

func TestRejectForgedPlanAndOverlappingRoots(t *testing.T) {
	w,p,a,_,_,root:=fixture(t)
	for _,candidate:=range []string{"../outside.pdf","/tmp/other.pdf","Prints/other.pdf","Prints/2026/CON.pdf","Prints/2026/other.txt"} {
		bad:=p; bad.RelativePath=candidate
		if _,err:=w.Publish(bad,a);err==nil { t.Errorf("accepted %q",candidate) }
	}
	if _,err:=NewWorker(root,root);err==nil { t.Fatal("accepted identical roots") }
	if err:=os.Mkdir(filepath.Join(root,"child"),0700);err!=nil { t.Fatal(err) }
	if _,err:=NewWorker(root,filepath.Join(root,"child"));err==nil { t.Fatal("accepted nested roots") }
}

func TestConcurrentPublishExactlyOneVisible(t *testing.T) {
	w,p,a,_,_,_:=fixture(t)
	const count=12
	var group sync.WaitGroup
	results:=make(chan error,count)
	for i:=0;i<count;i++ {
		group.Add(1)
		go func(){ defer group.Done(); _,err:=w.Publish(p,a); results<-err }()
	}
	group.Wait();close(results)
	success,collision:=0,0
	for err:=range results {
		switch {
		case err==nil: success++
		case errors.Is(err,ErrCollision): collision++
		default: t.Errorf("unexpected concurrent failure: %v",err)
		}
	}
	if success!=1 || collision!=count-1 { t.Fatalf("success=%d collision=%d",success,collision) }
}

func TestInvalidSourceIdentityAndByteLimit(t *testing.T) {
	w,p,a,_,_,_:=fixture(t)
	for _,bad:=range []Artifact{{SHA256:"../bad",Bytes:0},{SHA256:strings.ToUpper(a.SHA256),Bytes:a.Bytes},{SHA256:a.SHA256,Bytes:-1},{SHA256:a.SHA256,Bytes:1<<30+1}} {
		if _,err:=w.Publish(p,bad);err==nil { t.Fatalf("accepted %+v",bad) }
	}
}

func TestInspectRefusesUntrustedReceipt(t *testing.T) {
	w,_,a,_,_,_:=fixture(t)
	for _,bad:=range []string{"../../outside.pdf","/absolute.pdf","..\\backslash.pdf","CON.pdf"} {
		if _,err:=w.Inspect(Receipt{RelativePath:bad,SHA256:a.SHA256,Bytes:a.Bytes});err==nil { t.Errorf("accepted forged receipt %q",bad) }
	}
}


func TestExplicitGroupWritableModeAndPrivateDefault(t *testing.T) {
	w,p,a,_,_,root:=fixture(t)
	first,err:=w.Publish(p,a)
	if err!=nil {t.Fatal(err)}
	if runtime.GOOS!="windows" {
		st,err:=os.Stat(filepath.Join(root,filepath.FromSlash(first.RelativePath)))
		if err!=nil || st.Mode().Perm()!=0o600 {t.Fatalf("private default mode=%v err=%v",st,err)}
	}
	if runtime.GOOS=="windows" {return} // Win32 ACL mapping requires its own operator qualification.
	base:=t.TempDir()
	canon:=filepath.Join(base,"canonical")
	exportBase:=filepath.Join(base,"export-base")
	shareDir:=filepath.Join(exportBase,"shared")
	if err:=os.MkdirAll(canon,0700);err!=nil {t.Fatal(err)}
	if err:=os.MkdirAll(shareDir,0700);err!=nil {t.Fatal(err)}
	if err:=os.Chmod(shareDir,0770);err!=nil {t.Fatal(err)}
	canonicalFile:=filepath.Join(canon,"sha256",a.SHA256[:2],a.SHA256)
	if err:=os.MkdirAll(filepath.Dir(canonicalFile),0700);err!=nil {t.Fatal(err)}
	originalData:=[]byte("%PDF-1.7\ncontrolled fixture\n")
	if err:=os.WriteFile(canonicalFile,originalData,0600);err!=nil {t.Fatal(err)}
	groupWorker,err:=NewWorkerWithMode(canon,exportBase,0o660)
	if err!=nil {t.Fatal(err)}
	defer groupWorker.Close()
	gp:=p
	gp.Folder="shared/"+p.Folder
	gp.RelativePath="shared/"+p.RelativePath
	rcpt,err:=groupWorker.Publish(gp,a)
	if err!=nil {t.Fatal(err)}
	st,err:=os.Stat(filepath.Join(exportBase,filepath.FromSlash(rcpt.RelativePath)))
	if err!=nil || st.Mode().Perm()!=0o660 {t.Fatalf("group mode=%v err=%v",st,err)}
	dir,err:=os.Stat(filepath.Join(shareDir,"Prints","2026"))
	if err!=nil || dir.Mode().Perm()!=0o770 {t.Fatalf("share subfolder mode=%v err=%v",dir,err)}
	stage,err:=os.Stat(filepath.Join(exportBase,".foliorelay-private-stage"))
	if err!=nil || stage.Mode().Perm()!=0o700 {t.Fatalf("private staging mode=%v err=%v",stage,err)}
	if _,err:=NewWorkerWithMode(canon,exportBase,0o666);err==nil {t.Fatal("world-writable mode accepted")}
	// Protect against the old regression: a shared export root cannot be the
	// root of the worker's staging filesystem, even when its files use 0660.
	if _,err:=NewWorkerWithMode(canon,shareDir,0660);!errors.Is(err,ErrUnsafeShare) {
		t.Fatalf("accepted SMB writable staging parent: %v",err)
	}
	if _,err:=groupWorker.Publish(p,a);!errors.Is(err,ErrUnsafeShare) {
		t.Fatalf("group output escaped shared subtree: %v",err)
	}
}
