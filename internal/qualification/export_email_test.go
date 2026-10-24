package qualification

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	frExport "github.com/SemperSupra/folio-relay/internal/export"
	frOutbox "github.com/SemperSupra/folio-relay/internal/outbox"
	"github.com/SemperSupra/folio-relay/internal/security"
)

func TestExportEmailIndependentDurableEffects(t *testing.T) {
	root:=t.TempDir()
	canon:=filepath.Join(root,"canonical")
	shared:=filepath.Join(root,"shared")
	if err:=os.MkdirAll(canon,0700);err!=nil {t.Fatal(err)}
	if err:=os.MkdirAll(shared,0700);err!=nil {t.Fatal(err)}
	body:=[]byte("%PDF-1.7\ncombined qualification fixture\n")
	sum:=sha256.Sum256(body)
	digest:=hex.EncodeToString(sum[:])
	src:=filepath.Join(canon,"sha256",digest[:2],digest)
	if err:=os.MkdirAll(filepath.Dir(src),0700);err!=nil {t.Fatal(err)}
	if err:=os.WriteFile(src,body,0600);err!=nil {t.Fatal(err)}
	worker,err:=frExport.NewWorker(canon,shared)
	if err!=nil {t.Fatal(err)}
	defer worker.Close()
	jobID:="job-"+strings.Repeat("b",64)
	plan,err:=frExport.MakePlan(
		frExport.Defaults{Folder:"Finance/2026",FilenameTemplate:"{job_name}-{job_id}"},
		frExport.Overrides{},
		frExport.Job{JobID:jobID,SourceTitle:"Quarterly Report",AcceptedAt:time.Date(2026,10,24,1,2,3,0,time.UTC),MediaType:"application/pdf"},
	)
	if err!=nil {t.Fatal(err)}
	artifact:=frExport.Artifact{SHA256:digest,Bytes:int64(len(body))}
	exportFile:=filepath.Join(root,"export-journal.frj")
	mailFile:=filepath.Join(root,"mail-journal.frj")
	ex,err:=frExport.OpenLedger(exportFile)
	if err!=nil {t.Fatal(err)}
	mail,err:=frOutbox.Open(mailFile)
	if err!=nil {t.Fatal(err)}
	intent:=frExport.Intent{ActionID:"user-export-action-1",JobID:jobID,Plan:plan,Artifact:artifact}
	effect,err:=ex.Enqueue(intent)
	if err!=nil {t.Fatal(err)}
	if _,err:=ex.Begin(effect.ID);err!=nil {t.Fatal(err)}
	receipt,err:=worker.Publish(plan,artifact)
	if err!=nil {t.Fatal(err)}
	if _,err:=ex.Complete(effect.ID,true);err!=nil {t.Fatal(err)}
	emailEffect,err:=mail.Enqueue(frOutbox.Request{
		Recipient:"recipient@example.net",ArtifactSHA256:digest,
		SenderProfileID:"test-provider",RouteLegID:"mail-route-1",SubjectPolicyID:"default",
	},security.EmailRecipientPolicy{})
	if err!=nil {t.Fatal(err)}
	if _,err:=mail.Begin(emailEffect.ID);err!=nil {t.Fatal(err)}
	// A fake post-DATA connection loss must not replay successful export.
	if _,err:=mail.Conclude(emailEffect.ID,"delivery_unknown");err!=nil {t.Fatal(err)}
	if err:=ex.Close();err!=nil {t.Fatal(err)}
	if err:=mail.Close();err!=nil {t.Fatal(err)}

	// Simulate a named SMB client renaming its copy. The canonical source,
	// export durable record, and independent email leg must remain intact.
	out:=filepath.Join(shared,filepath.FromSlash(receipt.RelativePath))
	if err:=os.Rename(out,out+".renamed-by-smb");err!=nil {t.Fatal(err)}
	ex2,err:=frExport.OpenLedger(exportFile)
	if err!=nil {t.Fatal(err)}
	defer ex2.Close()
	mail2,err:=frOutbox.Open(mailFile)
	if err!=nil {t.Fatal(err)}
	defer mail2.Close()
	stored,err:=ex2.Get(effect.ID)
	if err!=nil || stored.State!="published" {t.Fatalf("export state: %#v %v",stored,err)}
	state,err:=worker.Inspect(receipt)
	if err!=nil || state!="missing_or_moved" {t.Fatalf("SMB state: %q %v",state,err)}
	msg,err:=mail2.Get(emailEffect.ID)
	if err!=nil || msg.State!="delivery_unknown" {t.Fatalf("mail state: %#v %v",msg,err)}
	if _,err:=mail2.Begin(emailEffect.ID);!errors.Is(err,frOutbox.ErrUnresolvedDelivery) {t.Fatalf("must not resend uncertain email: %v",err)}
	if _,err:=ex2.Begin(effect.ID);!errors.Is(err,frExport.ErrReconcile) {t.Fatalf("must not replay export: %v",err)}
	// Only a new explicit action may generate another copy.
	intent.ActionID="user-export-action-2"
	next,err:=ex2.Enqueue(intent)
	if err!=nil || next.ID==effect.ID {t.Fatalf("explicit reexport intent: %#v %v",next,err)}
	if _,err:=ex2.Begin(next.ID);err!=nil {t.Fatal(err)}
	if _,err:=worker.Publish(plan,artifact);err!=nil {t.Fatal(err)}
	if _,err:=ex2.Complete(next.ID,true);err!=nil {t.Fatal(err)}
	b,err:=os.ReadFile(src)
	if err!=nil || string(b)!=string(body) {t.Fatal("canonical artifact modified by SMB or outbox effect")}
}
