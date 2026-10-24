package paperless

import (
    "context"
    "errors"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

func syntheticAuthorizedReviewOracle(ctx context.Context,
    requested ReviewAccessSubject) (ReviewAccessVerdict, error) {
    if ctx.Err() != nil ||
        requested.ActorID != "reviewer-a" ||
        requested.OperatorID != "trusted-issuer" ||
        requested.TargetID != "paperless-main" ||
        requested.DocumentID != 48 ||
        requested.ApprovalID != "user-approved-001" {
        return ReviewAccessVerdict{}, ErrArchivedGrantPolicy
    }
    return ReviewAccessVerdict{
        Subject: requested,
        ActorIdentityVerified: true,
        OperatorIdentityVerified: true,
        RecordPermissionVerified: true,
        Allowed: true,
    }, nil
}

func TestArchivedPolicyAdmitsExactTrustedActorAndEffect(t *testing.T) {
    outbox, archived := makeArchivedSyntheticReview(t)
    seen := 0
    decide, err := NewArchivedGrantDecision(outbox, "paperless-main",
        func(ctx context.Context, subject ReviewAccessSubject) (ReviewAccessVerdict, error) {
            seen++
            if subject.Action != GrantIssue ||
                subject.EffectID != archived.ID ||
                subject.CanonicalSHA256 != archived.Intent.CanonicalSHA256 ||
                subject.PolicyVersion != archived.Intent.PolicyVersion ||
                subject.TaskID != archived.TaskID ||
                subject.DocumentID != 48 {
                t.Fatal("oracle was given a substituted or unarchived receipt")
            }
            return syntheticAuthorizedReviewOracle(ctx, subject)
        })
    if err != nil {t.Fatal(err)}
    if err := decide(context.Background(), GrantIssue,
        "reviewer-a", archived.ID, "trusted-issuer"); err != nil {
        t.Fatal("correct positive record permission denied:", err)
    }
    if seen != 1 {t.Fatalf("expected single authoritative decision, got %d", seen)}
}

func TestArchivedPolicyDeniesUnarchivedOrCrossTargetBeforeOracle(t *testing.T) {
    outbox, archived := makeArchivedSyntheticReview(t)
    other, err := outbox.Enqueue(Intent{
        CanonicalSHA256: strings.Repeat("b", 64),
        TargetID: "paperless-main", PolicyVersion: "v1",
        ApprovalID: "unverified-second",
    })
    if err != nil {t.Fatal(err)}
    reads := 0
    sentinelOracle := func(context.Context, ReviewAccessSubject) (ReviewAccessVerdict, error) {
        reads++
        return ReviewAccessVerdict{Allowed: true}, nil
    }
    valid, err := NewArchivedGrantDecision(outbox, "paperless-main", sentinelOracle)
    if err != nil {t.Fatal(err)}
    wrongTarget, err := NewArchivedGrantDecision(outbox, "paperless-other", sentinelOracle)
    if err != nil {t.Fatal(err)}
    cases := []struct{
        name string
        fn ReviewGrantDecision
        action ReviewGrantAction
        actor, effect, operator string
    }{
        {"approved-but-not-archived",valid,GrantIssue,"reviewer-a",other.ID,"trusted-issuer"},
        {"unknown-effect",valid,GrantIssue,"reviewer-a",strings.Repeat("f",64),"trusted-issuer"},
        {"wrong-native-target",wrongTarget,GrantIssue,"reviewer-a",archived.ID,"trusted-issuer"},
        {"invalid-action",valid,"grant-all","reviewer-a",archived.ID,"trusted-issuer"},
        {"bad-actor",valid,GrantIssue,"../../other",archived.ID,"trusted-issuer"},
        {"bad-operator",valid,GrantIssue,"reviewer-a",archived.ID,"bad operator"},
        {"invalid-effect",valid,GrantIssue,"reviewer-a","fake","trusted-issuer"},
    }
    for _, tc := range cases {
        t.Run(tc.name,func(t *testing.T){
            if err := tc.fn(context.Background(), tc.action,
                tc.actor,tc.effect,tc.operator); !errors.Is(err,ErrArchivedGrantPolicy) {
                t.Fatalf("unqualified request reached issuer: %v",err)
            }
        })
    }
    if reads != 0 {t.Fatalf("called authorization oracle for invalid/unarchived effect %d times",reads)}
}

func TestArchivedPolicyDeniesFakeOrStaleAuthorizationVerdicts(t *testing.T) {
    outbox, archived := makeArchivedSyntheticReview(t)
    var cases = []struct{
        name string
        alter func(*ReviewAccessVerdict)
    }{
        {"not-allowed",func(v *ReviewAccessVerdict){v.Allowed=false}},
        {"actor-not-authenticated",func(v *ReviewAccessVerdict){v.ActorIdentityVerified=false}},
        {"operator-not-authenticated",func(v *ReviewAccessVerdict){v.OperatorIdentityVerified=false}},
        {"record-permission-unverified",func(v *ReviewAccessVerdict){v.RecordPermissionVerified=false}},
        {"substituted-actor",func(v *ReviewAccessVerdict){v.Subject.ActorID="someone-else"}},
        {"substituted-operator",func(v *ReviewAccessVerdict){v.Subject.OperatorID="someone-else"}},
        {"substituted-effect",func(v *ReviewAccessVerdict){v.Subject.EffectID=strings.Repeat("c",64)}},
        {"substituted-target",func(v *ReviewAccessVerdict){v.Subject.TargetID="another-dms"}},
        {"substituted-document",func(v *ReviewAccessVerdict){v.Subject.DocumentID=2}},
        {"substituted-content",func(v *ReviewAccessVerdict){v.Subject.CanonicalSHA256=strings.Repeat("b",64)}},
        {"substituted-approval",func(v *ReviewAccessVerdict){v.Subject.ApprovalID="fake"}},
        {"substituted-task",func(v *ReviewAccessVerdict){v.Subject.TaskID="other"}},
        {"substituted-action",func(v *ReviewAccessVerdict){v.Subject.Action=GrantRevoke}},
    }
    for _, tc := range cases {
        t.Run(tc.name,func(t *testing.T) {
            decide, err := NewArchivedGrantDecision(outbox,"paperless-main",
                func(_ context.Context, subject ReviewAccessSubject)(ReviewAccessVerdict,error){
                    v:=ReviewAccessVerdict{Subject:subject,
                        ActorIdentityVerified:true,
                        OperatorIdentityVerified:true,
                        RecordPermissionVerified:true,
                        Allowed:true}
                    tc.alter(&v)
                    return v,nil
                })
            if err!=nil{t.Fatal(err)}
            if err:=decide(context.Background(),GrantIssue,"reviewer-a",
                archived.ID,"trusted-issuer");!errors.Is(err,ErrArchivedGrantPolicy){
                t.Fatalf("forged authorization receipt admitted: %v",err)
            }
        })
    }
    for _,entry:=range []struct{actor,operator string}{
        {"reviewer-b","trusted-issuer"},
        {"reviewer-a","unauthenticated-operator"},
    }{
        decide,err:=NewArchivedGrantDecision(outbox,"paperless-main",syntheticAuthorizedReviewOracle)
        if err!=nil{t.Fatal(err)}
        if err:=decide(context.Background(),GrantIssue,entry.actor,archived.ID,
            entry.operator);!errors.Is(err,ErrArchivedGrantPolicy){
            t.Fatalf("identity substitution admitted: %#v %v",entry,err)
        }
    }
    denied:=errors.New("independent identity system unavailable")
    decide,err:=NewArchivedGrantDecision(outbox,"paperless-main",
        func(context.Context,ReviewAccessSubject)(ReviewAccessVerdict,error){
            return ReviewAccessVerdict{},denied
        })
    if err!=nil{t.Fatal(err)}
    if err:=decide(context.Background(),GrantIssue,"reviewer-a",archived.ID,
        "trusted-issuer");!errors.Is(err,ErrArchivedGrantPolicy){
        t.Fatal("unavailable external oracle permitted fallback")
    }
}

func TestArchivedPolicyCannotBeCreatedWithImplicitAuthority(t *testing.T) {
    outbox,_:=makeArchivedSyntheticReview(t)
    for _,arg:=range []struct{
        outbox *Outbox
        target string
        oracle ReviewArchivedAccessOracle
    }{
        {nil,"paperless-main",syntheticAuthorizedReviewOracle},
        {outbox,"bad target",syntheticAuthorizedReviewOracle},
        {outbox,"paperless-main",nil},
    }{
        if _,err:=NewArchivedGrantDecision(arg.outbox,arg.target,arg.oracle);
            !errors.Is(err,ErrArchivedGrantPolicy) {
            t.Fatal("missing target/owner/actor authority was allowed")
        }
    }
}

func TestArchivedPolicyBoundToAnchoredLedgerIssueAndRevoke(t *testing.T){
    outbox, archived:=makeArchivedSyntheticReview(t)
    policy,err:=NewArchivedGrantDecision(outbox,"paperless-main",
        func(ctx context.Context,subject ReviewAccessSubject)(ReviewAccessVerdict,error){
            if subject.ActorID!="reviewer-a"||subject.OperatorID!="trusted-issuer"||
               subject.EffectID!=archived.ID {
                return ReviewAccessVerdict{},ErrArchivedGrantPolicy
            }
            return ReviewAccessVerdict{Subject:subject,
                ActorIdentityVerified:true, OperatorIdentityVerified:true,
                RecordPermissionVerified:true,Allowed:true},nil
        })
    if err!=nil{t.Fatal(err)}
    private:=filepath.Join(t.TempDir(),"private-grants")
    if err:=os.Mkdir(private,0o700);err!=nil{t.Fatal(err)}
    path:=filepath.Join(private,"grants.frj")
    anchor:=&reviewAnchorFake{}
    now:=func()time.Time{return time.Date(2026,10,24,20,0,0,0,time.UTC)}
    key:=[]byte("0123456789abcdef0123456789abcdef") // synthetic test only
    ledger,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,policy,now,anchor)
    if err!=nil{t.Fatal(err)}
    if _,err:=ledger.Issue(context.Background(),"reviewer-b",archived.ID,
        "trusted-issuer",time.Minute);!errors.Is(err,ErrReviewGrantLedger){
        t.Fatal("other actor could mint cross-person review access")
    }
    if anchor.current.Sequence!=0 {t.Fatal("unauthorized operation advanced anchor")}
    token,err:=ledger.Issue(context.Background(),"reviewer-a",archived.ID,
        "trusted-issuer",10*time.Minute)
    if err!=nil||token==""{t.Fatalf("approved actor denied: %v",err)}
    hash:=digestGrant(t,token)
    if anchor.current.Sequence!=1{t.Fatal("checkpoint not committed before bearer returned")}
    if err:=ledger.Close();err!=nil{t.Fatal(err)}
    restart,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,policy,now,anchor)
    if err!=nil{t.Fatal(err)}
    defer restart.Close()
    if _,err:=restart.Lookup(hash);err!=nil{t.Fatalf("trusted user lost grant after restart: %v",err)}
    if err:=restart.Revoke(context.Background(),hash,"other-operator");!errors.Is(err,ErrReviewGrantLedger){
        t.Fatalf("untrusted actor revoked grant: %v",err)
    }
    if anchor.current.Sequence!=1 {t.Fatal("rejected revoke advanced anchor")}
    if err:=restart.Revoke(context.Background(),hash,"trusted-issuer");err!=nil{t.Fatal(err)}
    if anchor.current.Sequence!=2 {t.Fatal("revocation not independently checkpointed")}
    found,err:=restart.Lookup(hash)
    if err!=nil||!found.Revoked {t.Fatalf("revoked actor access survived: %#v %v",found,err)}
}
