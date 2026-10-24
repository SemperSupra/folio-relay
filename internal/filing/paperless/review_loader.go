package paperless

import (
 "context"
 "errors"
)

// ErrReviewUnavailable intentionally contains no document identifiers,
// backend detail, credential material or native OCR text.
var ErrReviewUnavailable = errors.New("filing review unavailable")

// ReviewLanguageLookup is an application-owned, trustworthy annotation.
// OCR/LLM cannot assert a user's preferred UI language or authorize a route.
// Return ReviewUnknown if the record language has not been adjudicated.
type ReviewLanguageLookup func(context.Context,string)(ReviewLanguage,error)

// NewArchivedReviewLoader is a *concrete read-only* link between an immutable,
// archived filing effect in the FolioRelay durable outbox and the Paperless API
// v3.1.3. A ReviewHandler MUST perform per-effect auth BEFORE calling it.
//
// This loader checks that the effect's intended DMS target matches the
// operator-configured target, and that the remote document ID is exactly the
// durable archived receipt. It uses only GET operations; it NEVER exports raw
// canonical bytes, creates DMS items, changes metadata or triggers native AI.
func NewArchivedReviewLoader(outbox *Outbox,client *Client,targetID string,language ReviewLanguageLookup)(ReviewLoader,error){
 if outbox==nil||outbox.durable==nil||client==nil||
    !identRe.MatchString(targetID)||language==nil{
  return nil,ErrReviewUnavailable
 }
 return func(ctx context.Context,effect string)(ReviewEnvelope,error){
  if !shaRe.MatchString(effect){return ReviewEnvelope{},ErrReviewUnavailable}
  ticket,err:=outbox.Get(effect)
  if err!=nil||ticket.State!="archived"||ticket.DocumentID<=0||
     ticket.ID!=effect||ticket.Intent.Validate()!=nil||
     effectID(ticket.Intent)!=ticket.ID||ticket.Intent.TargetID!=targetID{
    return ReviewEnvelope{},ErrReviewUnavailable
  }
  selectedLanguage,err:=language(ctx,effect)
  if err!=nil{return ReviewEnvelope{},ErrReviewUnavailable}
  // The remote token is scoped by the operator-configured Paperless Client,
  // which rejects redirects and unapproved non-HTTPS hosts. It is never
  // attached to review JSON/HTML or returned as a browser navigation token.
  native,err:=client.ReadRecord(ctx,ticket.DocumentID)
  if err!=nil{return ReviewEnvelope{},ErrReviewUnavailable}
  hints,err:=client.ReadClassicSuggestions(ctx,ticket.DocumentID)
  if err!=nil{return ReviewEnvelope{},ErrReviewUnavailable}
  packet,err:=BuildReviewEnvelope(ticket,native,hints,selectedLanguage)
  if err!=nil{return ReviewEnvelope{},ErrReviewUnavailable}
  return packet,nil
 },nil
}
