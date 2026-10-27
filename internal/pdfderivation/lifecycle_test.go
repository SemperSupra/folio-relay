package pdfderivation

import (
    "strings"
    "testing"
)

func testSpec(kind OutputKind) Spec {
    return Spec{
        SourceSHA256:hash("untouched source"),
        SourceMIME:SourceURF,
        ProcessorProfile:"synthetic-ocr/2026-10-27",
        OutputKind:kind,
    }
}
func testEvents(key string) []JobEvent {
    return []JobEvent{
        {ID:"begin",Key:key,Sequence:1,Action:Start},
    }
}

func TestDerivationKeyStableAndDomainSeparated(t *testing.T) {
    s:=testSpec(SearchablePDF)
    key,err:=s.Key()
    if err!=nil{t.Fatal(err)}
    repeated,err:=s.Key()
    if err!=nil || key!=repeated{t.Fatal("nondeterministic derivation key")}
    if !strings.HasPrefix(key,"pdf-derivation/v1/") {t.Fatalf("unexpected namespace: %q",key)}
    s.OutputKind=EditablePDF
    alternative,err:=s.Key()
    if err!=nil || alternative==key {t.Fatal("output kind was not isolated")}
    s.OutputKind=SearchablePDF
    s.ProcessorProfile="synthetic-ocr/2026-10-28"
    changed,err:=s.Key()
    if err!=nil || changed==key {t.Fatal("profile change did not version derivation")}
    for _,bad:=range []string{""," a","bad\nversion",strings.Repeat("x",181)} {
        s:=testSpec(SearchablePDF)
        s.ProcessorProfile=bad
        if _,err:=s.Key();err==nil{t.Fatalf("invalid profile admitted %q",bad)}
    }
}

func TestDerivationReadyRequiresVerifiedMatchingArtifact(t *testing.T) {
    spec:=testSpec(SearchablePDF)
    key,_:=spec.Key()
    result:=fixture(SourceURF,SearchableImage)
    result.ProcessorProfile=spec.ProcessorProfile
    events:=append(testEvents(key),JobEvent{
        ID:"complete",Key:key,Sequence:2,Action:Complete,Receipt:&result,
    })
    state,err:=Replay(spec,events)
    if err!=nil{t.Fatal(err)}
    if state.State!=JobReady || state.Revision!=2 || len(state.NextActions)!=0 {
        t.Fatalf("incorrect terminal projection: %+v",state)
    }
    identical:=append(append([]JobEvent{},events...),events[1])
    replay,err:=Replay(spec,identical)
    if err!=nil || replay.Revision!=2 {t.Fatalf("idempotent replay failed: %v",err)}

    edited:=append([]JobEvent{},events...)
    edited[1].ID="reused-event-id"
    edited[1].Receipt=&result
    edited=append(edited,JobEvent{ID:"reused-event-id",Key:key,Sequence:2,
        Action:Fail,ReasonCode:"inconsistent"})
    if _,err:=Replay(spec,edited);err==nil || err.Error()!="idempotency_conflict" {
        t.Fatalf("inconsistent duplicate event accepted: %v",err)
    }
    wrongProfile:=append([]JobEvent{},events...)
    incorrect:=result
    incorrect.ProcessorProfile="other-profile"
    wrongProfile[1].Receipt=&incorrect
    if _,err:=Replay(spec,wrongProfile);err==nil{
        t.Fatal("cross-profile result was promoted")
    }
    wrongSpec:=testSpec(EditablePDF)
    wrongKey,_:=wrongSpec.Key()
    wrongTier:=append(testEvents(wrongKey),JobEvent{ID:"done",Key:wrongKey,
        Sequence:2,Action:Complete,Receipt:&result})
    if _,err:=Replay(wrongSpec,wrongTier);err==nil{
        t.Fatal("searchable-only OCR misrepresented as editable derivative")
    }
}

func TestDerivationRedBlueTransitions(t *testing.T) {
    spec:=testSpec(SearchablePDF)
    key,_:=spec.Key()
    start:=testEvents(key)
    review:=JobEvent{ID:"review",Key:key,Sequence:2,Action:RequireReview,ReasonCode:"ocr_uncertain"}
    working:=append(append([]JobEvent{},start...),review)
    p,err:=Replay(spec,working)
    if err!=nil || p.State!=JobNeedsReview || len(p.NextActions)!=2 {
        t.Fatalf("review state failed: %+v %v",p,err)
    }
    working=append(working,JobEvent{ID:"resubmit",Key:key,Sequence:3,Action:Start})
    working=append(working,JobEvent{ID:"fail",Key:key,Sequence:4,Action:Fail,ReasonCode:"missing_ocr_tool"})
    p,err=Replay(spec,working)
    if err!=nil || p.State!=JobFailed || len(p.NextActions)!=0 {
        t.Fatalf("failed state unexpectedly actionable: %+v %v",p,err)
    }
    working=append(working,JobEvent{ID:"retry_after_failure",Key:key,Sequence:5,Action:Start})
    if _,err:=Replay(spec,working);err==nil {t.Fatal("failed job silently retried")}
    if _,err:=Replay(spec,[]JobEvent{{ID:"out_of_order",Key:key,
        Sequence:2,Action:Start}});err==nil {t.Fatal("gap accepted")}
    cancel,err:=Replay(spec,[]JobEvent{{ID:"stop",Key:key,Sequence:1,Action:Cancel}})
    if err!=nil || cancel.State!=JobCancelled || len(cancel.NextActions)!=0 {
        t.Fatalf("cancellation not terminal: %+v %v",cancel,err)
    }
}
