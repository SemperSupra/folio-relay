package bench

import (
 "math"
 "testing"
)

func TestBenchmarkMakesAbstentionsAndWrongRoutesVisible(t *testing.T){
 samples:=[]Sample{
  {CaseID:"invoice",ExpectedTypeID:4,SuggestedTypeIDs:[]int{4},AppliedTypeID:4,LatencyMilliseconds:100},
  {CaseID:"contract",ExpectedTypeID:5,SuggestedTypeIDs:[]int{4},AppliedTypeID:4,LatencyMilliseconds:200,HumanCorrections:1},
  {CaseID:"letter",ExpectedTypeID:3,SuggestedTypeIDs:[]int{},AppliedTypeID:0,LatencyMilliseconds:300,HumanCorrections:1},
  {CaseID:"ambiguous",ExpectedTypeID:6,SuggestedTypeIDs:[]int{3,6},AppliedTypeID:0,LatencyMilliseconds:400,HumanCorrections:1},
 }
 got,err:=Evaluate(samples)
 if err!=nil{t.Fatal(err)}
 if got.Total!=4||got.CorrectTopOne!=1||got.WrongTopOne!=1||got.Abstained!=1||got.Ambiguous!=1||got.WrongAutomaticRoutes!=1||got.HumanCorrections!=3||got.P95LatencyMilliseconds!=400 {t.Fatalf("bad metrics %+v",got)}
 if math.Abs(got.Accuracy-0.25)>0.001||math.Abs(got.Coverage-0.5)>0.001||math.Abs(got.PrecisionOnProposals-0.5)>0.001{t.Fatalf("misleading denominators %+v",got)}
}
func TestBenchmarkRejectsCherryPickedOrInvalidCases(t *testing.T){
 for _,tc:=range [][]Sample{
  nil,
  {{CaseID:"",ExpectedTypeID:1}},
  {{CaseID:"dup",ExpectedTypeID:1},{CaseID:"dup",ExpectedTypeID:2}},
  {{CaseID:"wrong",ExpectedTypeID:0}},
  {{CaseID:"negative",ExpectedTypeID:1,LatencyMilliseconds:-1}},
  {{CaseID:"duplicate-choice",ExpectedTypeID:1,SuggestedTypeIDs:[]int{1,1}}},
 }{
  if _,err:=Evaluate(tc);err==nil{t.Fatalf("accepted invalid fixture %+v",tc)}
 }
}
