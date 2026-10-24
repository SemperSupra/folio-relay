package paperless

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "strings"
 "unicode"
 "unicode/utf8"
)

// A correction is NOT an authorization to mutate Paperless. This package
// only constructs an inert preview that another, separately reviewed actor
// might present to a human. It has NO HTTP PATCH, database or file writes.
var (
 ErrCorrectionInvalid = errors.New("filing correction preview invalid")
 ErrCorrectionStale = errors.New("filing correction review snapshot changed")
)

type CorrectionActor struct {
 ActorID string
 MayPropose bool // supplied only by a trusted, audited auth subsystem
}

// TaxonomyAllowlist must be supplied from an independently authorized read
// of the current DMS taxonomy under the same tenant/target as the review.
// Neither OCR content nor model suggestions can add permitted identifiers.
type TaxonomyAllowlist struct {
 DocumentTypeIDs []int
 CorrespondentIDs []int
 StoragePathIDs []int
 TagIDs []int
}

// A nil pointer means unchanged; non-nil is an explicit new value.
// Empty TagIDs explicitly clears tags. Zero/negative remote IDs are refused.
type CorrectionChoice struct {
 Title *string `json:"title,omitempty"`
 DocumentTypeID *int `json:"document_type_id,omitempty"`
 CorrespondentID *int `json:"correspondent_id,omitempty"`
 StoragePathID *int `json:"storage_path_id,omitempty"`
 TagIDs *[]int `json:"tag_ids,omitempty"`
}

type CorrectionRequest struct {
 ExpectedReviewFingerprint string
 Choice CorrectionChoice
}

type CorrectionPreview struct {
 EffectID string `json:"effect_id"`
 PaperlessDocumentID int `json:"paperless_document_id"`
 ActorID string `json:"actor_id"`
 ReviewFingerprint string `json:"review_fingerprint"`
 ProposalFingerprint string `json:"proposal_fingerprint"`
 OriginalTitle string `json:"original_title"`
 ProposedTitle string `json:"proposed_title"`
 OriginalDocumentTypeID *int `json:"original_document_type_id,omitempty"`
 ProposedDocumentTypeID *int `json:"proposed_document_type_id,omitempty"`
 OriginalCorrespondentID *int `json:"original_correspondent_id,omitempty"`
 ProposedCorrespondentID *int `json:"proposed_correspondent_id,omitempty"`
 OriginalStoragePathID *int `json:"original_storage_path_id,omitempty"`
 ProposedStoragePathID *int `json:"proposed_storage_path_id,omitempty"`
 OriginalTagIDs []int `json:"original_tag_ids"`
 ProposedTagIDs []int `json:"proposed_tag_ids"`
 RequiresHumanApproval bool `json:"requires_human_approval"`
 MayWriteRemotely bool `json:"may_write_remotely"`
}

// ReviewFingerprint is a snapshot of *native assigned metadata*, not the
// canonical artifact SHA (which must not change). An external commit must
// fetch Paperless's current record again and compare this fingerprint;
// even that preflight alone is NOT atomic concurrency protection.
func ReviewFingerprint(p ReviewEnvelope)(string,error) {
 if !allowedEnvelope(p.EffectID,p){return "",ErrCorrectionInvalid}
 snapshot:=struct {
  EffectID string
  PaperlessDocumentID int
  NativeTitle string
  NativeDocumentType *int
  NativeCorrespondent *int
  NativeStoragePath *int
  NativeTags []int
 }{p.EffectID,p.PaperlessDocumentID,p.NativeTitle,p.NativeDocumentType,
   p.NativeCorrespondent,p.NativeStoragePath,p.NativeTags}
 bytes,err:=json.Marshal(snapshot)
 if err!=nil{return "",ErrCorrectionInvalid}
 sum:=sha256.Sum256(append([]byte("foliorelay-filing-review-snapshot/v1\x00"),bytes...))
 return hex.EncodeToString(sum[:]),nil
}
func safeCorrectionTitle(v string)bool {
 if !utf8.ValidString(v)||len([]rune(v))==0||len([]rune(v))>200||
    strings.TrimSpace(v)=="" {return false}
 for _,r:=range v {
   if unicode.IsControl(r)||r>=0x202A&&r<=0x202E||r>=0x2066&&r<=0x2069 {return false}
 }
 return true
}
func containsAllowed(ids []int,want int)bool {
 if want<=0{return false}
 for _,id:=range ids {if id==want{return true}}
 return false
}
func duplicateOrBadIDs(ids []int)bool {
 seen:=map[int]bool{}
 if len(ids)>1000{return true}
 for _,id:=range ids{
   if id<=0||seen[id]{return true}
   seen[id]=true
 }
 return false
}
func copyPointer(v *int)*int {
 if v==nil{return nil}
 x:=*v
 return &x
}
func intsEqual(a,b []int)bool {
 if len(a)!=len(b){return false}
 for i:=range a{if a[i]!=b[i]{return false}}
 return true
}

// PlanCorrection may produce a visible proposal, NEVER a remote operation.
// All IDs must be real existing DMS taxonomy values admitted by a separate
// trusted allowlist. No LLM title/path strings are trusted as instructions.
func PlanCorrection(p ReviewEnvelope,actor CorrectionActor,allow TaxonomyAllowlist,req CorrectionRequest)(CorrectionPreview,error) {
 if !actor.MayPropose||!identRe.MatchString(actor.ActorID)||actor.ActorID=="" ||
    !allowedEnvelope(p.EffectID,p) {return CorrectionPreview{},ErrCorrectionInvalid}
 original,err:=ReviewFingerprint(p)
 if err!=nil{return CorrectionPreview{},err}
 if !shaRe.MatchString(req.ExpectedReviewFingerprint)||req.ExpectedReviewFingerprint!=original {
   return CorrectionPreview{},ErrCorrectionStale
 }
 for _,list:=range [][]int{allow.DocumentTypeIDs,allow.CorrespondentIDs,allow.StoragePathIDs,allow.TagIDs}{
   if duplicateOrBadIDs(list){return CorrectionPreview{},ErrCorrectionInvalid}
 }
 v:=req.Choice
 proposal:=CorrectionPreview{
   EffectID:p.EffectID,PaperlessDocumentID:p.PaperlessDocumentID,
   ActorID:actor.ActorID,ReviewFingerprint:original,
   OriginalTitle:p.NativeTitle,ProposedTitle:p.NativeTitle,
   OriginalDocumentTypeID:copyPointer(p.NativeDocumentType),
   ProposedDocumentTypeID:copyPointer(p.NativeDocumentType),
   OriginalCorrespondentID:copyPointer(p.NativeCorrespondent),
   ProposedCorrespondentID:copyPointer(p.NativeCorrespondent),
   OriginalStoragePathID:copyPointer(p.NativeStoragePath),
   ProposedStoragePathID:copyPointer(p.NativeStoragePath),
   OriginalTagIDs:append([]int(nil),p.NativeTags...),
   ProposedTagIDs:append([]int(nil),p.NativeTags...),
   RequiresHumanApproval:true,MayWriteRemotely:false,
 }
 changed:=false
 if v.Title!=nil{
   if !safeCorrectionTitle(*v.Title){return CorrectionPreview{},ErrCorrectionInvalid}
   proposal.ProposedTitle=*v.Title
   if proposal.ProposedTitle!=p.NativeTitle {changed=true}
 }
 for _,t:=range []struct{from *int;choices []int;to **int}{
   {v.DocumentTypeID,allow.DocumentTypeIDs,&proposal.ProposedDocumentTypeID},
   {v.CorrespondentID,allow.CorrespondentIDs,&proposal.ProposedCorrespondentID},
   {v.StoragePathID,allow.StoragePathIDs,&proposal.ProposedStoragePathID},
 }{
   if t.from==nil{continue}
   if !containsAllowed(t.choices,*t.from){return CorrectionPreview{},ErrCorrectionInvalid}
   if *t.to==nil||**t.to!=*t.from{changed=true}
   *t.to=copyPointer(t.from)
 }
 if v.TagIDs!=nil {
   if duplicateOrBadIDs(*v.TagIDs){return CorrectionPreview{},ErrCorrectionInvalid}
   for _,id:=range *v.TagIDs {
      if !containsAllowed(allow.TagIDs,id){return CorrectionPreview{},ErrCorrectionInvalid}
   }
   proposal.ProposedTagIDs=append([]int(nil),(*v.TagIDs)...)
   if !intsEqual(proposal.ProposedTagIDs,p.NativeTags){changed=true}
 }
 if !changed{return CorrectionPreview{},ErrCorrectionInvalid}
 // Canonical encoding stabilizes the requested choice and the review
 // precondition across repeated UI/CLI/agent previews.
 b,err:=json.Marshal(struct{
   ActorID string
   ReviewFingerprint string
   Title string
   Type *int
   Correspondent *int
   StoragePath *int
   Tags []int
 }{actor.ActorID,original,proposal.ProposedTitle,proposal.ProposedDocumentTypeID,
 proposal.ProposedCorrespondentID,proposal.ProposedStoragePathID,proposal.ProposedTagIDs})
 if err!=nil{return CorrectionPreview{},ErrCorrectionInvalid}
 sum:=sha256.Sum256(append([]byte("foliorelay-filing-correction-preview/v1\x00"),b...))
 proposal.ProposalFingerprint=hex.EncodeToString(sum[:])
 return proposal,nil
}
