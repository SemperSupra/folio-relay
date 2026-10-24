package outbox

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SemperSupra/folio-relay/internal/security"
)

func sample() Request {
	return Request{
		Recipient:"person@example.com",
		ArtifactSHA256:strings.Repeat("a",64),
		SenderProfileID:"icloud",
		RouteLegID:"leg-001",
		SubjectPolicyID:"subject-v1",
	}
}
func makeOutbox(t *testing.T) (*Outbox,string) {
	t.Helper()
	file:=filepath.Join(t.TempDir(),"mail","journal.frj")
	o,err:=Open(file)
	if err!=nil { t.Fatal(err) }
	t.Cleanup(func(){o.Close()})
	return o,file
}

func TestStableEffectAndDuplicateSuppression(t *testing.T) {
	o,_:=makeOutbox(t)
	req:=sample()
	first,err:=o.Enqueue(req,security.EmailRecipientPolicy{})
	if err!=nil { t.Fatal(err) }
	again,err:=o.Enqueue(req,security.EmailRecipientPolicy{})
	if err!=nil || first!=again || first.State!="pending" { t.Fatalf("idempotence %#v %#v %v",first,again,err) }
	req.RouteLegID="second"
	other,err:=o.Enqueue(req,security.EmailRecipientPolicy{})
	if err!=nil || other.ID==first.ID { t.Fatalf("new route reused effect id: %v",err) }
}

func TestSuccessIsTransportAcceptanceOnly(t *testing.T) {
	o,_:=makeOutbox(t)
	e,err:=o.Enqueue(sample(),security.EmailRecipientPolicy{})
	if err!=nil { t.Fatal(err) }
	start,err:=o.Begin(e.ID)
	if err!=nil || start.State!="dispatching" { t.Fatalf("begin %#v %v",start,err) }
	if _,err:=o.Begin(e.ID);!errors.Is(err,ErrUnresolvedDelivery) { t.Fatalf("duplicate dispatch should stop: %v",err) }
	final,err:=o.Conclude(e.ID,"delivered")
	if err!=nil || final.State!="delivered" { t.Fatalf("finish %#v %v",final,err) }
	if _,err:=o.Begin(e.ID);!errors.Is(err,ErrUnresolvedDelivery) { t.Fatalf("transport delivered must not retry: %v",err) }
}

func TestAmbiguousCrashRefusesAutomatedRetry(t *testing.T) {
	o,file:=makeOutbox(t)
	e,err:=o.Enqueue(sample(),security.EmailRecipientPolicy{})
	if err!=nil { t.Fatal(err) }
	if _,err:=o.Begin(e.ID);err!=nil { t.Fatal(err) }
	if err:=o.Close();err!=nil { t.Fatal(err) }
	recovered,err:=Open(file)
	if err!=nil { t.Fatal(err) }
	defer recovered.Close()
	item,err:=recovered.Get(e.ID)
	if err!=nil || item.State!="delivery_unknown" { t.Fatalf("crashed dispatch must be unknown %#v %v",item,err) }
	if _,err:=recovered.Begin(e.ID);!errors.Is(err,ErrUnresolvedDelivery) { t.Fatalf("re-dispatch after crash: %v",err) }
	if _,err:=recovered.Conclude(e.ID,"delivered");!errors.Is(err,ErrUnresolvedDelivery) { t.Fatalf("unproven conclusion: %v",err) }
	final,err:=recovered.ResolveUnknown(e.ID,"delivered","confirmed via provider audit")
	if err!=nil || final.State!="delivered" { t.Fatalf("operator resolution %#v %v",final,err) }
	if _,err:=recovered.Begin(e.ID);!errors.Is(err,ErrUnresolvedDelivery) { t.Fatalf("resolved mail must not re-dispatch: %v",err) }
}

func TestProviderFailureAndAmbiguity(t *testing.T) {
	for _,verdict:=range []string{"failed","delivery_unknown"} {
		t.Run(verdict,func(t *testing.T){
			o,_:=makeOutbox(t)
			e,err:=o.Enqueue(sample(),security.EmailRecipientPolicy{})
			if err!=nil { t.Fatal(err) }
			if _,err:=o.Begin(e.ID);err!=nil { t.Fatal(err) }
			final,err:=o.Conclude(e.ID,verdict)
			if err!=nil || final.State!=verdict { t.Fatalf("%#v %v",final,err) }
			if _,err:=o.Begin(e.ID);err==nil { t.Fatal("resubmitted terminal effect") }
		})
	}
}

func TestRecipientAndProfileAuthority(t *testing.T) {
	o,_:=makeOutbox(t)
	for _,mailbox:=range []string{"x@example.com,b@example.com","X <x@example.com>","x@example.com\r\nBCC:foo@example.org",""," person@example.com"} {
		req:=sample();req.Recipient=mailbox
		if _,err:=o.Enqueue(req,security.EmailRecipientPolicy{});err==nil { t.Errorf("allowed %q",mailbox) }
	}
	req:=sample()
	if _,err:=o.Enqueue(req,security.EmailRecipientPolicy{AllowedDomains:map[string]struct{}{"another.net":{}}});err==nil { t.Fatal("wrong domain accepted") }
	req.SenderProfileID="../evil"
	if _,err:=o.Enqueue(req,security.EmailRecipientPolicy{});err==nil { t.Fatal("bad profile accepted") }
	req=sample();req.ArtifactSHA256="bad"
	if _,err:=o.Enqueue(req,security.EmailRecipientPolicy{});err==nil { t.Fatal("bad artifact accepted") }
}

func TestOperatorResolutionRequiresExplicitEvidence(t *testing.T) {
	o,_:=makeOutbox(t)
	e,err:=o.Enqueue(sample(),security.EmailRecipientPolicy{})
	if err!=nil { t.Fatal(err) }
	if _,err:=o.ResolveUnknown(e.ID,"delivered","some evidence");err==nil { t.Fatal("resolved pending without dispatch") }
	if _,err:=o.Begin(e.ID);err!=nil { t.Fatal(err) }
	if _,err:=o.ResolveUnknown(e.ID,"delivered","premature");err==nil { t.Fatal("resolved actively dispatching effect") }
	if _,err:=o.Conclude(e.ID,"delivery_unknown");err!=nil { t.Fatal(err) }
	if _,err:=o.ResolveUnknown(e.ID,"delivered","");err==nil { t.Fatal("allowed no evidence") }
	if _,err:=o.ResolveUnknown(e.ID,"failed","unproven\r\ninjection");err==nil { t.Fatal("allowed header control evidence") }
	final,err:=o.ResolveUnknown(e.ID,"failed","provider log confirms reject")
	if err!=nil || final.State!="failed" { t.Fatalf("failed manual resolution %#v %v",final,err) }
}

func TestDistinctRecipientsDifferentEffects(t *testing.T) {
	o,_:=makeOutbox(t)
	a,err:=o.Enqueue(sample(),security.EmailRecipientPolicy{})
	if err!=nil { t.Fatal(err) }
	req:=sample();req.Recipient="different@example.com"
	b,err:=o.Enqueue(req,security.EmailRecipientPolicy{})
	if err!=nil || a.ID==b.ID { t.Fatalf("recipient isolation %#v %#v %v",a,b,err) }
}
