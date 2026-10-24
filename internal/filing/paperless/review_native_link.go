package paperless

import (
 "fmt"
 "net"
 "net/url"
 "strings"
)

// NativeReviewURL builds a token-free link to Paperless's own authenticated
// document detail UI. Exact Paperless v3.1.3 Angular route: documents/:id.
// The origin can only come from a previously operator-approved Client built
// by NewClient; no title, OCR, tag, LLM or request URL is ever interpolated.
func NativeReviewURL(c *Client,review ReviewEnvelope)(string,error){
 if c==nil||!allowedEnvelope(review.EffectID,review){return "",ErrReviewEndpoint}
 origin,err:=url.Parse(c.endpoint)
 if err!=nil||origin.Host==""||origin.User!=nil||origin.RawQuery!=""||
   origin.Fragment!=""||(origin.Path!=""&&origin.Path!="/") {
    return "",ErrReviewEndpoint
 }
 if origin.Scheme!="https" {
    ip:=net.ParseIP(origin.Hostname())
    if origin.Scheme!="http"||!((ip!=nil&&ip.IsLoopback())||strings.EqualFold(origin.Hostname(),"localhost")) {
      return "",ErrReviewEndpoint
    }
 }
 link:=origin.ResolveReference(&url.URL{
   Path:fmt.Sprintf("/documents/%d",review.PaperlessDocumentID),
 })
 // Do not permit credentials, embedded query strings or fragments in links.
 if link.User!=nil||link.RawQuery!=""||link.Fragment!=""{return "",ErrReviewEndpoint}
 return link.String(),nil
}
