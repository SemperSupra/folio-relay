package paperless

import (
 "context"
 "fmt"
 "net/http"
 "testing"
)

func TestClassicSuggestionsReadOnly(t *testing.T){
 count:=0
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){
   count++
   if r.URL.Path!="/api/documents/47/suggestions/"||r.Method!="GET" {t.Fatalf("unexpected method/path %s %s",r.Method,r.URL.Path)}
   fmt.Fprint(w,`{"correspondents":[7],"tags":[11,12],"document_types":[3],"storage_paths":[2],"dates":["2026-10-24"]}`)
 })
 v,err:=c.ReadClassicSuggestions(context.Background(),47)
 if err!=nil||count!=1||len(v.Tags)!=2||v.DocumentTypes[0]!=3{t.Fatalf("suggestions %#v %v",v,err)}
 if _,err:=c.ReadClassicSuggestions(context.Background(),-1);err!=ErrInvalid{t.Fatalf("invalid input reached network: %v",err)}
}

func TestClassicSuggestionsBadJSONHolds(t *testing.T){
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,"not-json")})
 if _,err:=c.ReadClassicSuggestions(context.Background(),47);err!=ErrUncertain{t.Fatalf("accepted malformed response: %v",err)}
}
