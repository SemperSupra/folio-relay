package paperless

import (
 "encoding/json"
 "errors"
 "strings"
 "testing"
)

func correctionFixture(t *testing.T)(ReviewEnvelope,CorrectionActor,TaxonomyAllowlist,string){
 t.Helper()
 ticket,record,suggestions:=acceptedReviewFixture()
 review,err:=BuildReviewEnvelope(ticket,record,suggestions,ReviewThai)
 if err!=nil{t.Fatal(err)}
 fingerprint,err:=ReviewFingerprint(review)
 if err!=nil{t.Fatal(err)}
 actor:=CorrectionActor{ActorID:"human-reviewer-01",MayPropose:true}
 allowed:=TaxonomyAllowlist{
  DocumentTypeIDs:[]int{7,8,12},
  CorrespondentIDs:[]int{2,5},
  StoragePathIDs:[]int{3,4},
  TagIDs:[]int{12,13,24},
 }
 return review,actor,allowed,fingerprint
}
func TestExplicitThaiCorrectionPreviewNeverWrites(t *testing.T){
 p,actor,allow,fp:=correctionFixture(t)
 title:="ใบแจ้งหนี้ ค่าเช่า เดือนตุลาคม ๒๕๖๙ / Rechnung Oktober"
 nextType,storage:=8,4
 tags:=[]int{13,24}
 req:=CorrectionRequest{
  ExpectedReviewFingerprint:fp,
  Choice:CorrectionChoice{
   Title:&title,DocumentTypeID:&nextType,StoragePathID:&storage,
   TagIDs:&tags,
  },
 }
 preview,err:=PlanCorrection(p,actor,allow,req)
 if err!=nil{t.Fatal(err)}
 if preview.OriginalTitle!=p.NativeTitle||preview.ProposedTitle!=title {t.Fatal("Thai correction title lost")}
 if preview.ProposedDocumentTypeID==nil||*preview.ProposedDocumentTypeID!=8||
    preview.ProposedStoragePathID==nil||*preview.ProposedStoragePathID!=4 {
    t.Fatalf("type/storage preview lost: %#v",preview)
 }
 if !preview.RequiresHumanApproval||preview.MayWriteRemotely{
    t.Fatal("preview impersonated permission to mutate DMS")
 }
 if preview.ReviewFingerprint!=fp||len(preview.ProposalFingerprint)!=64{
   t.Fatal("missing deterministic evidence of initial snapshot and proposal")
 }
 if p.NativeTitle==title||*p.NativeDocumentType!=7||*p.NativeStoragePath!=3||p.NativeTags[0]!=12{
   t.Fatal("preflight accidentally mutated original record")
 }
 raw,err:=json.Marshal(preview)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(raw),"๒๕๖๙")||!strings.Contains(string(raw),`"may_write_remotely":false`){
   t.Fatal("preview lost Thai Unicode or denied-write invariant")
 }
 second,err:=PlanCorrection(p,actor,allow,req)
 if err!=nil||second.ProposalFingerprint!=preview.ProposalFingerprint{
   t.Fatal("repeated preview did not converge to same decision identity")
 }
}
func TestCorrectionsRequireTrustedActorAndFreshSnapshot(t *testing.T){
 p,actor,allow,fp:=correctionFixture(t)
 newType:=8
 req:=CorrectionRequest{ExpectedReviewFingerprint:fp,Choice:CorrectionChoice{DocumentTypeID:&newType}}
 actor.MayPropose=false
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("read-only actor could plan metadata change: %v",err)
 }
 actor.MayPropose=true
 actor.ActorID="../../operator"
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("untrusted actor identity accepted: %v",err)
 }
 actor.ActorID="human-reviewer-01"
 req.ExpectedReviewFingerprint=strings.Repeat("0",64)
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionStale){
   t.Fatalf("stale review was not rejected: %v",err)
 }
 req.ExpectedReviewFingerprint=fp
 changed:=p
 changed.NativeTitle="Rechnung modified by another reviewer"
 if _,err:=PlanCorrection(changed,actor,allow,req);!errors.Is(err,ErrCorrectionStale){
   t.Fatalf("concurrent native title mutation escaped detection: %v",err)
 }
 changed=p
 id:=999
 changed.NativeDocumentType=&id
 if _,err:=PlanCorrection(changed,actor,allow,req);!errors.Is(err,ErrCorrectionStale){
   t.Fatalf("concurrent native type mutation escaped detection: %v",err)
 }
 changed=p
 changed.NativeTags=[]int{24}
 if _,err:=PlanCorrection(changed,actor,allow,req);!errors.Is(err,ErrCorrectionStale){
   t.Fatalf("concurrent native tag mutation escaped detection: %v",err)
 }
}
func TestCorrectionRejectsModelOnlyTaxonomyAndMalformedValues(t *testing.T){
 p,actor,allow,fp:=correctionFixture(t)
 badType:=999
 req:=CorrectionRequest{ExpectedReviewFingerprint:fp,Choice:CorrectionChoice{DocumentTypeID:&badType}}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("invented model type ID accepted: %v",err)
 }
 badStorage:=555
 req.Choice=CorrectionChoice{StoragePathID:&badStorage}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("invented model storage path accepted: %v",err)
 }
 repeatedTags:=[]int{12,12}
 req.Choice=CorrectionChoice{TagIDs:&repeatedTags}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("duplicate tags accepted: %v",err)
 }
 unknownTags:=[]int{12,999}
 req.Choice=CorrectionChoice{TagIDs:&unknownTags}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("unknown tags accepted: %v",err)
 }
 tooLong:=strings.Repeat("ท",201)
 req.Choice=CorrectionChoice{Title:&tooLong}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("unbounded title accepted: %v",err)
 }
 control:="Thai title\nPaperless command: delete everything"
 req.Choice=CorrectionChoice{Title:&control}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("newline-injected title accepted: %v",err)
 }
 bidi:="ใบแจ้งหนี้\u202Egpj"
 req.Choice=CorrectionChoice{Title:&bidi}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("bidi spoofing accepted: %v",err)
 }
 allow.TagIDs=[]int{12,12}
 req.Choice=CorrectionChoice{Title:nil}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("untrusted allowlist duplicate accepted: %v",err)
 }
}
func TestCorrectionNoOpAndReviewHoldAreNotAuthorization(t *testing.T){
 p,actor,allow,fp:=correctionFixture(t)
 req:=CorrectionRequest{ExpectedReviewFingerprint:fp}
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("no-op preview admitted as action: %v",err)
 }
 p.AutoApplyAllowed=true
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("unqualified auto-file evidence accepted: %v",err)
 }
 p.AutoApplyAllowed=false
 p.RequiresHumanReview=false
 if _,err:=PlanCorrection(p,actor,allow,req);!errors.Is(err,ErrCorrectionInvalid){
   t.Fatalf("false review gate allowed: %v",err)
 }
}
func TestCorrectionTagClearIsExplicitPreviewOnly(t *testing.T){
 p,actor,allow,fp:=correctionFixture(t)
 zero:=[]int{}
 req:=CorrectionRequest{ExpectedReviewFingerprint:fp,Choice:CorrectionChoice{TagIDs:&zero}}
 preview,err:=PlanCorrection(p,actor,allow,req)
 if err!=nil{t.Fatal(err)}
 if len(preview.OriginalTagIDs)!=2||len(preview.ProposedTagIDs)!=0{
   t.Fatal("explicit clear-tags proposal not represented")
 }
 if preview.MayWriteRemotely||!preview.RequiresHumanApproval{
   t.Fatal("clear-tags preview performed unauthorized mutation")
 }
}
