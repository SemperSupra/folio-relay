//go:build paperless_integration

package paperless

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "errors"

 "time"
)

// synthPDF constructs a valid one-page PDF with deterministic selectable text.
// No personal or licensed input is used in the disposable end-to-end test.
func synthPDF(content string) []byte {
 escaped:=strings.NewReplacer("\\","\\\\","(","\\(",")","\\)","\r"," ","\n"," ").Replace(content)
 stream:=fmt.Sprintf("BT /F1 18 Tf 50 740 Td (%s) Tj ET\n",escaped)
 objects:=[]string{
  "<< /Type /Catalog /Pages 2 0 R >>",
  "<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
  fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream",len(stream),stream),
 }
 var pdf bytes.Buffer
 pdf.WriteString("%PDF-1.4\n")
 offsets:=[]int{0}
 for index,obj:=range objects {
    offsets=append(offsets,pdf.Len())
    fmt.Fprintf(&pdf,"%d 0 obj\n%s\nendobj\n",index+1,obj)
 }
 xref:=pdf.Len()
 fmt.Fprintf(&pdf,"xref\n0 %d\n0000000000 65535 f \n",len(objects)+1)
 for _,v:=range offsets[1:] {fmt.Fprintf(&pdf,"%010d 00000 n \n",v)}
 fmt.Fprintf(&pdf,"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",len(objects)+1,xref)
 return pdf.Bytes()
}

func TestDisposableRealPaperless313(t *testing.T){
 addr:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_URL")
 token:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_TOKEN")
 if addr==""||token==""{t.Skip("only explicitly activated disposable hosted Paperless test")}
 c,err:=NewClient(Config{BaseURL:addr,Token:token,TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 content:=synthPDF("TEST INVOICE 2026-10-24 COMPANY ALPHA DEMO ONLY")
 sum:=sha256.Sum256(content)
 doc:=Document{Content:content,SHA256:hex.EncodeToString(sum[:]),MediaType:"application/pdf"}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute)
 defer cancel()
 t0:=time.Now()
 journalPath:=filepath.Join(t.TempDir(),"filing.frj")
 outbox,err:=OpenOutbox(journalPath)
 if err!=nil{t.Fatal(err)}
 defer outbox.Close()
 filing:=Intent{
   CanonicalSHA256:doc.SHA256,
   TargetID:"paperless-fixture",
   PolicyVersion:"v1",
   ApprovalID:"approved-fixture-1",
 }
 ticket,err:=outbox.Enqueue(filing)
 if err!=nil{t.Fatal(err)}
 if _,err:=outbox.Begin(ticket.ID);err!=nil{t.Fatal(err)}
 taskID,err:=c.Upload(ctx,doc)
 if err!=nil{t.Fatalf("upload into pinned fixture: %v",err)}
 if _,err:=outbox.Accept(ticket.ID,taskID);err!=nil{t.Fatalf("durable task receipt: %v",err)}
 t.Logf("synthetic bytes=%d task accepted=true",len(doc.Content))
 var observed Task
 for {
  if err:=ctx.Err();err!=nil{t.Fatalf("timed out waiting for Paperless task: %v last_status=%q last_outcome=%q",err,observed.Status,observed.Outcome)}
  observed,err=c.Observe(ctx,taskID)
  if err!=nil{t.Fatalf("task API v10 contract: %v",err)}
  if observed.Outcome=="archived"||observed.Outcome=="duplicate"||observed.Outcome=="rejected" {break}
  time.Sleep(2*time.Second)
 }
 if observed.Outcome!="archived"||observed.DocumentID<=0 {
    t.Fatalf("Paperless did not archive valid generated PDF, status=%s outcome=%s",observed.Status,observed.Outcome)
 }
 if final,err:=outbox.MarkTask(ticket.ID,observed);err!=nil||final.State!="archived"{
   t.Fatalf("durable archive receipt %#v: %v",final,err)
 }
 record,err:=c.ReadRecord(ctx,observed.DocumentID)
 if err!=nil{t.Fatalf("native metadata readback: %v",err)}
 if record.ID!=observed.DocumentID||record.Title=="" {t.Fatalf("invalid native record: %#v",record)}
 classic,err:=c.ReadClassicSuggestions(ctx,observed.DocumentID)
 if err!=nil {t.Fatalf("native classic suggestions contract: %v",err)}
 t.Logf("archived=true native_title_present=true tags=%d classic_type_candidates=%d seconds=%.2f",len(record.Tags),len(classic.DocumentTypes),time.Since(t0).Seconds())
 // Confirm that a restarted actor sees the previously committed result.
 if err:=outbox.Close();err!=nil{t.Fatal(err)}
 recovery,err:=OpenOutbox(journalPath)
 if err!=nil{t.Fatal(err)}
 defer recovery.Close()
 recovered,err:=recovery.Get(ticket.ID)
 if err!=nil||recovered.State!="archived"||recovered.DocumentID!=record.ID{
   t.Fatalf("durable end-to-end recovery %#v %v",recovered,err)
 }
 // First qualify the *concrete*, per-target, read-only loader against
 // the actual pinned Paperless container and the restarted durable outbox.
 sourceLoader,err:=NewArchivedReviewLoader(recovery,c,"paperless-fixture",
   func(_ context.Context,effect string)(ReviewLanguage,error){
     if effect!=ticket.ID{return ReviewUnknown,ErrReviewUnavailable}
     return ReviewEnglish,nil
   })
 if err!=nil{t.Fatal(err)}
 loadedPacket,err:=sourceLoader(ctx,ticket.ID)
 if err!=nil||loadedPacket.PaperlessDocumentID!=record.ID||
   loadedPacket.EffectID!=ticket.ID||loadedPacket.AutoApplyAllowed||
   !loadedPacket.RequiresHumanReview {
   t.Fatalf("concrete archived Paperless GET loader unsafe: %#v %v",loadedPacket,err)
 }
 t.Log("archived_paperless_native_readonly_loader=true auto_apply=false")

 // This verifies the NEW archived-effect identity policy with the SAME
 // actual pinned Paperless 3.1.3 record. The authorization oracle and anchor
 // are synthetic test doubles and do not establish production admission.
 ownerPolicy,err:=NewArchivedGrantDecision(recovery,"paperless-fixture",
  func(_ context.Context,sub ReviewAccessSubject)(ReviewAccessVerdict,error){
   if sub.Action!=GrantIssue&&sub.Action!=GrantRevoke{return ReviewAccessVerdict{},ErrArchivedGrantPolicy}
   if sub.ActorID!="approved-paperless-reviewer"||
      sub.OperatorID!="synthetic-policy"||
      sub.EffectID!=ticket.ID||sub.DocumentID!=record.ID||
      sub.TargetID!="paperless-fixture"||sub.TaskID!=taskID||
      sub.CanonicalSHA256!=doc.SHA256||sub.ApprovalID!=filing.ApprovalID {
     return ReviewAccessVerdict{},ErrArchivedGrantPolicy
   }
   return ReviewAccessVerdict{Subject:sub,ActorIdentityVerified:true,
    OperatorIdentityVerified:true,RecordPermissionVerified:true,Allowed:true},nil
  })
 if err!=nil{t.Fatal(err)}
 gatedDir:=filepath.Join(t.TempDir(),"anchored-policy-fixture")
 if err:=os.Mkdir(gatedDir,0o700);err!=nil{t.Fatal(err)}
 gatedAnchor:=&reviewAnchorFake{}
 gatedLedger,err:=OpenAnchoredEncryptedReviewGrantLedger(
  filepath.Join(gatedDir,"grants.frj"),
  []byte("0123456789abcdef0123456789abcdef"),ownerPolicy,time.Now,gatedAnchor)
 if err!=nil{t.Fatal(err)}
 defer gatedLedger.Close()
 if _,err:=gatedLedger.Issue(ctx,"other-record-user",ticket.ID,
  "synthetic-policy",time.Minute);err==nil{
   t.Fatal("unapproved actor obtained a review bearer")
 }
 if gatedAnchor.current.Sequence!=0{t.Fatal("unapproved grant advanced independent checkpoint")}
 gatedToken,err:=gatedLedger.Issue(ctx,"approved-paperless-reviewer",ticket.ID,
  "synthetic-policy",time.Minute*10)
 if err!=nil{t.Fatal(err)}
 gatedBrowser,err:=NewReviewBrowserHandler(gatedLedger.Lookup,sourceLoader,
  "https://reviews.example.test",[]byte(strings.Repeat("g",32)),time.Now)
 if err!=nil{t.Fatal(err)}
 gatedPost:=httptest.NewRequest("POST","https://reviews.example.test/filing/review-sessions",
   strings.NewReader(fmt.Sprintf("{\\\"effect_id\\\":%q}",ticket.ID)))
 gatedPost.Header.Set("Authorization","Bearer "+gatedToken)
 gatedPost.Header.Set("Origin","https://reviews.example.test")
 gatedPost.Header.Set("X-FolioRelay-Review-Session","1")
 gatedPost.Header.Set("Content-Type","application/json")
 gatedResp:=httptest.NewRecorder()
 gatedBrowser.ServeHTTP(gatedResp,gatedPost)
 if gatedResp.Code!=201||len(gatedResp.Result().Cookies())!=1{
   t.Fatalf("authorized archived document browser session denied: HTTP %d",gatedResp.Code)
 }
 gatedGet:=httptest.NewRequest("GET","https://reviews.example.test/filing/reviews/"+ticket.ID,nil)
 gatedGet.AddCookie(gatedResp.Result().Cookies()[0])
 validGated:=httptest.NewRecorder()
 gatedBrowser.ServeHTTP(validGated,gatedGet)
 if validGated.Code!=200||!strings.Contains(validGated.Body.String(),record.Title){
   t.Fatalf("authorized archived Paperless record unavailable HTTP %d",validGated.Code)
 }
 if err:=gatedLedger.Revoke(ctx,digestGrant(t,gatedToken),"synthetic-policy");err!=nil{
  t.Fatal(err)
 }
 deniedGated:=httptest.NewRecorder()
 gatedBrowser.ServeHTTP(deniedGated,gatedGet)
 if deniedGated.Code!=401||gatedAnchor.current.Sequence!=2{
   t.Fatalf("revoked browser session survived archived-bound checkpoint: %d",deniedGated.Code)
 }
 t.Log("real_paperless_archived_policy_anchor_browser=true issued_one=true revoked=true remote_patch=false")

 // Exercise the TLS-bound, short-lived browser session using the same REAL
 // Paperless 3.1.3 native archived document (still completely synthetic).
 browserToken,browserHash:=capabilityFixture()
 browserNow:=time.Now().UTC()
 browserGrant:=ReviewGrant{TokenHash:browserHash,ActorID:"human-browser-fixture",
   EffectID:ticket.ID,ValidFrom:browserNow.Add(-time.Minute),
   ExpiresAt:browserNow.Add(time.Hour),CanRead:true}
 browserHandler,err:=NewReviewBrowserHandler(
   func(hash [32]byte)(ReviewGrant,error){
     if hash!=browserHash{return ReviewGrant{},ErrReviewGrant}
     return browserGrant,nil
   },sourceLoader,"https://reviews.example.test",
   []byte("0123456789abcdef0123456789abcdef"),
   func()time.Time{return browserNow})
 if err!=nil{t.Fatal(err)}
 sessionRequest:=httptest.NewRequest("POST","https://reviews.example.test/filing/review-sessions",
   strings.NewReader(fmt.Sprintf("{\"effect_id\":%q}",ticket.ID)))
 sessionRequest.Header.Set("Authorization","Bearer "+browserToken)
 sessionRequest.Header.Set("Origin","https://reviews.example.test")
 sessionRequest.Header.Set("X-FolioRelay-Review-Session","1")
 sessionRequest.Header.Set("Content-Type","application/json")
 sessionResult:=httptest.NewRecorder()
 browserHandler.ServeHTTP(sessionResult,sessionRequest)
 if sessionResult.Code!=201||len(sessionResult.Result().Cookies())!=1{
    t.Fatalf("synthetic browser exchange failed: status=%d",sessionResult.Code)
 }
 readRequest:=httptest.NewRequest("GET","https://reviews.example.test/filing/reviews/"+ticket.ID,nil)
 readRequest.AddCookie(sessionResult.Result().Cookies()[0])
 readResult:=httptest.NewRecorder()
 browserHandler.ServeHTTP(readResult,readRequest)
 if readResult.Code!=200||!strings.Contains(readResult.Body.String(),record.Title) {
    t.Fatalf("browser reviewer could not read authorized real-Paperless document: status=%d",readResult.Code)
 }
 browserGrant.Revoked=true
 revokedResult:=httptest.NewRecorder()
 browserHandler.ServeHTTP(revokedResult,readRequest)
 if revokedResult.Code!=401{
   t.Fatalf("revoked native Paperless browser record still accessible: %d",revokedResult.Code)
 }
 t.Log("archived_paperless_to_browser_scoped_session=true revoked_before_dms_read=true auto_apply=false")

 // Replay the SAME synthetic document through an encrypted, durable and
 // revoked grant instead of a throwaway memory lookup. Token issuance is
 // explicitly admitted only by this disposable fixture policy.
 grantDir:=filepath.Join(t.TempDir(),"private-grant-state")
 if err:=os.Mkdir(grantDir,0o700);err!=nil{t.Fatal(err)}
 grantFile:=filepath.Join(grantDir,"encrypted-review-grants.frj")
 ledgerKey:=[]byte("0123456789abcdef0123456789abcdef")
 admit:=func(_ context.Context,action ReviewGrantAction,actor,effect,operator string)error{
   if (action!=GrantIssue&&action!=GrantRevoke)||actor!="synthetic-human"||
      effect!=ticket.ID||operator!="synthetic-policy" {return ErrReviewGrantLedger}
   return nil
 }
 durableGrants,err:=OpenEncryptedReviewGrantLedger(grantFile,ledgerKey,admit,time.Now)
 if err!=nil{t.Fatal(err)}
 issued,err:=durableGrants.Issue(ctx,"synthetic-human",ticket.ID,"synthetic-policy",30*time.Minute)
 if err!=nil{t.Fatal(err)}
 issuedHash:=digestGrant(t,issued)
 if err:=durableGrants.Close();err!=nil{t.Fatal(err)}
 recoveredGrants,err:=OpenEncryptedReviewGrantLedger(grantFile,ledgerKey,admit,time.Now)
 if err!=nil{t.Fatal(err)}
 defer recoveredGrants.Close()
 durableBrowser,err:=NewReviewBrowserHandler(recoveredGrants.Lookup,sourceLoader,
   "https://reviews.example.test",[]byte(strings.Repeat("k",32)),time.Now)
 if err!=nil{t.Fatal(err)}
 durablePOST:=httptest.NewRequest("POST","https://reviews.example.test/filing/review-sessions",
   strings.NewReader(fmt.Sprintf("{\"effect_id\":%q}",ticket.ID)))
 durablePOST.Header.Set("Authorization","Bearer "+issued)
 durablePOST.Header.Set("Origin","https://reviews.example.test")
 durablePOST.Header.Set("X-FolioRelay-Review-Session","1")
 durablePOST.Header.Set("Content-Type","application/json")
 sessionReply:=httptest.NewRecorder()
 durableBrowser.ServeHTTP(sessionReply,durablePOST)
 if sessionReply.Code!=201||len(sessionReply.Result().Cookies())!=1{
   t.Fatalf("restarted encrypted grant unable to open synthetic browser session: HTTP %d",sessionReply.Code)
 }
 durableGET:=httptest.NewRequest("GET","https://reviews.example.test/filing/reviews/"+ticket.ID,nil)
 durableGET.AddCookie(sessionReply.Result().Cookies()[0])
 validReply:=httptest.NewRecorder()
 durableBrowser.ServeHTTP(validReply,durableGET)
 if validReply.Code!=200||!strings.Contains(validReply.Body.String(),record.Title){
   t.Fatalf("real Paperless read via durable encrypted grant failed: HTTP %d",validReply.Code)
 }
 if err:=recoveredGrants.Revoke(ctx,issuedHash,"synthetic-policy");err!=nil{t.Fatal(err)}
 deniedReply:=httptest.NewRecorder()
 durableBrowser.ServeHTTP(deniedReply,durableGET)
 if deniedReply.Code!=401{
   t.Fatalf("revoked durable browser session exposed native Paperless: HTTP %d",deniedReply.Code)
 }
 t.Log("durable_encrypted_grants_paperless_real=true restart=true revoke=true no_remote_metadata_patch=true")

 // End-to-end, the REAL disposable Paperless metadata feeds our inert
 // reviewer API and a preview, without registering runtime routes or PATCH.
 packet,err:=BuildReviewEnvelope(recovered,record,classic,ReviewEnglish)
 if err!=nil{t.Fatalf("real native readback rejected for review: %v",err)}
 reviews:=0
 handler,err:=NewReviewHandler(
   func(_ *http.Request, effect string)(ReviewPrincipal,error){
     if effect!=packet.EffectID{return ReviewPrincipal{},ErrReview}
     return ReviewPrincipal{ActorID:"synthetic-fixture",CanRead:true},nil
   },
   func(_ context.Context,effect string)(ReviewEnvelope,error){
     reviews++
     if effect!=packet.EffectID{return ReviewEnvelope{},ErrReview}
     return packet,nil
   })
 if err!=nil{t.Fatal(err)}
 rr:=httptest.NewRecorder()
 handler.ServeHTTP(rr,httptest.NewRequest("GET","/api/filing/reviews/"+packet.EffectID,nil))
 if rr.Code!=200{t.Fatalf("real native review JSON not served: HTTP %d",rr.Code)}
 var reviewed ReviewEnvelope
 if err:=json.Unmarshal(rr.Body.Bytes(),&reviewed);err!=nil{t.Fatal(err)}
 if reviews!=1||reviewed.PaperlessDocumentID!=record.ID||
    !reviewed.RequiresHumanReview||reviewed.AutoApplyAllowed{
    t.Fatalf("real review contract unsafe: %#v calls=%d",reviewed,reviews)
 }
 fingerprint,err:=ReviewFingerprint(reviewed)
 if err!=nil{t.Fatal(err)}
 reviewedTitle:="Synthetic paperless invoice – review only"
 preview,err:=PlanCorrection(reviewed,CorrectionActor{
    ActorID:"synthetic-fixture",MayPropose:true,
 },TaxonomyAllowlist{},CorrectionRequest{
    ExpectedReviewFingerprint:fingerprint,
    Choice:CorrectionChoice{Title:&reviewedTitle},
 })
 if err!=nil||preview.MayWriteRemotely||!preview.RequiresHumanApproval{
   t.Fatalf("real native metadata preview unsafe: %#v err %v",preview,err)
 }
 actualAfter,err:=c.ReadRecord(ctx,record.ID)
 if err!=nil||actualAfter.Title!=record.Title{
   t.Fatalf("preview mutated DMS despite being read-only: %#v %v",actualAfter,err)
 }
 t.Log("native_paperless_to_review_html_json_to_preview=true writes_to_dms=false")
 // The first real v3.1.3 qualification showed that a raw second
 // POST of identical bytes can create a SECOND archive ID. Prevent that
 // POST at our durable adapter boundary, including after journal restart.
 filing.ApprovalID="approved-fixture-2"
 repeated,err:=recovery.Enqueue(filing)
 if err!=nil||repeated.ID!=ticket.ID||repeated.Intent.ApprovalID!="approved-fixture-1"{
   t.Fatalf("repeated approval must retain original effect/authorization: %#v %v",repeated,err)
 }
 if _,err:=recovery.Begin(ticket.ID);!errors.Is(err,ErrHold){
   t.Fatalf("recovered archived effect must not be sent again: %v",err)
 }
 t.Log("duplicate repeated approval blocked before network=true")
}
