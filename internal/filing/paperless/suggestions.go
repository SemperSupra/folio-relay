package paperless

import (
 "context"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
)

// Suggestions are advisory only. The DMS is the sole classifier and archive
// authority. This read-only endpoint never changes tags, paths or ACLs.
type Suggestions struct {
 Correspondents []int `json:"correspondents"`
 Tags []int `json:"tags"`
 DocumentTypes []int `json:"document_types"`
 StoragePaths []int `json:"storage_paths"`
 Dates []string `json:"dates"`
}

// ReadClassicSuggestions requests Paperless built-in rules/ML suggestions.
// It does not enable AI, call an LLM or automatically apply any suggestion.
func (c *Client) ReadClassicSuggestions(ctx context.Context,id int)(Suggestions,error){
 if id<=0{return Suggestions{},ErrInvalid}
 resp,err:=c.do(ctx,http.MethodGet,fmt.Sprintf("/api/documents/%d/suggestions/",id),"",nil)
 if err!=nil{return Suggestions{},err}
 defer resp.Body.Close()
 var value Suggestions
 if err:=json.NewDecoder(io.LimitReader(resp.Body,1<<20)).Decode(&value);err!=nil{return Suggestions{},ErrUncertain}
 return value,nil
}

// AISuggestions mirrors the current v3.1.3 Paperless AI schema. Reading this
// endpoint can invoke model inference, so this method is NOT exposed by the
// baseline connector until explicit local-provider/network qualification.
type AISuggestions struct {
 Title *string `json:"title"`
 Correspondents []int `json:"correspondents"`
 SuggestedCorrespondents []string `json:"suggested_correspondents"`
 Tags []int `json:"tags"`
 SuggestedTags []string `json:"suggested_tags"`
 DocumentTypes []int `json:"document_types"`
 SuggestedDocumentTypes []string `json:"suggested_document_types"`
 StoragePaths []int `json:"storage_paths"`
 SuggestedStoragePaths []string `json:"suggested_storage_paths"`
 Dates []string `json:"dates"`
}
