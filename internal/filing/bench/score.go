// Package bench measures whether native Paperless suggestions reduce filing work.
// It is not a classifier and never changes records or metadata.
package bench

import (
 "errors"
 "sort"
)

type Sample struct {
 CaseID string `json:"case_id"`
 ExpectedTypeID int `json:"expected_type_id"`
 SuggestedTypeIDs []int `json:"suggested_type_ids"`
 AppliedTypeID int `json:"applied_type_id"`
 HumanCorrections int `json:"human_corrections"`
 LatencyMilliseconds int64 `json:"latency_milliseconds"`
}

type Metrics struct {
 Total int `json:"total"`
 CorrectTopOne int `json:"correct_top_one"`
 WrongTopOne int `json:"wrong_top_one"`
 Abstained int `json:"abstained"`
 Ambiguous int `json:"ambiguous"`
 WrongAutomaticRoutes int `json:"wrong_automatic_routes"`
 HumanCorrections int `json:"human_corrections"`
 Accuracy float64 `json:"accuracy"`
 PrecisionOnProposals float64 `json:"precision_on_proposals"`
 Coverage float64 `json:"coverage"`
 P95LatencyMilliseconds int64 `json:"p95_latency_milliseconds"`
}

func Evaluate(samples []Sample)(Metrics,error){
 if len(samples)==0{return Metrics{},errors.New("no labeled samples")}
 out:=Metrics{Total:len(samples)}
 seen:=map[string]bool{}
 latencies:=make([]int64,0,len(samples))
 for _,s:=range samples{
   if s.CaseID==""||seen[s.CaseID]||s.ExpectedTypeID<=0||s.AppliedTypeID<0||s.HumanCorrections<0||s.LatencyMilliseconds<0 {
     return Metrics{},errors.New("invalid, missing or duplicate ground truth")
   }
   seen[s.CaseID]=true
   latencies=append(latencies,s.LatencyMilliseconds)
   out.HumanCorrections+=s.HumanCorrections
   if s.AppliedTypeID!=0 && s.AppliedTypeID!=s.ExpectedTypeID {out.WrongAutomaticRoutes++}
   switch len(s.SuggestedTypeIDs){
   case 0:out.Abstained++
   case 1:
     if s.SuggestedTypeIDs[0]<=0{return Metrics{},errors.New("invalid suggested ID")}
     if s.SuggestedTypeIDs[0]==s.ExpectedTypeID {out.CorrectTopOne++} else {out.WrongTopOne++}
   default:
     seenTypes:=map[int]bool{}
     for _,id:=range s.SuggestedTypeIDs{if id<=0||seenTypes[id]{return Metrics{},errors.New("invalid or repeated candidate")};seenTypes[id]=true}
     out.Ambiguous++
   }
 }
 out.Accuracy=float64(out.CorrectTopOne)/float64(out.Total)
 out.Coverage=float64(out.CorrectTopOne+out.WrongTopOne)/float64(out.Total)
 if out.CorrectTopOne+out.WrongTopOne>0 {
    out.PrecisionOnProposals=float64(out.CorrectTopOne)/float64(out.CorrectTopOne+out.WrongTopOne)
 }
 sort.Slice(latencies,func(i,j int)bool{return latencies[i]<latencies[j]})
 idx:=(95*len(latencies)+99)/100-1
 out.P95LatencyMilliseconds=latencies[idx]
 return out,nil
}
