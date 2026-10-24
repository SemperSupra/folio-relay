package paperless

import (
 "strings"
 "testing"
)

// thaiSpaceProjection is an EXPERIMENTAL, lossy, test-only projection of the
// exact synthetic Tesseract results. It does NOT modify Paperless content,
// canonical artifacts, tags, search indices or document types. Thai uses
// meaningful spaces between phrases; collapsing them may cause false matches.
// Never use this as an auto-filing authorization or data migration.
func thaiSpaceProjection(in string) string {
 r:=[]rune(in)
 thai:=func(v rune)bool{return v>=0x0e00&&v<=0x0e7f}
 var out []rune
 for i,v:=range r {
   if v==' ' && i>0 && i+1<len(r) && thai(r[i-1])&&thai(r[i+1]) {
     continue
   }
   out=append(out,v)
 }
 return string(out)
}

func TestSyntheticThaiOCRNormalizationShowsBenefitsAndHazards(t *testing.T) {
 cases:=[]struct{name,raw,canonical string}{
   {"invoice","ใบ แจ ้ ง ห น ี","ใบแจ้งหนี้"},
   {"insurance","ก ร ม ธร ร ม ์ ป ร ะ ก ั น ภั ย","กรมธรรม์ประกันภัย"},
   {"lease","ส ั ญ ญ า เช า","สัญญาเช่า"},
   {"appointment","ใบ น ั ด ห ม า ย","ใบนัดหมาย"},
   {"statement","ร า ย ก า ร เด ิ น บ ั ญ ช ี","รายการเดินบัญชี"},
   {"registration","ใบ ล ง ท ะ เบ ี ย น","ใบลงทะเบียน"},
 }
 exact:=0
 for _,sample:=range cases {
   if strings.Contains(thaiSpaceProjection(sample.raw),sample.canonical){exact++}
 }
 // The observed Tesseract output lost some Thai marks in invoice/lease.
 // Character-space collapse alone cannot repair those OCR errors.
 if exact!=4 {t.Fatalf("expected 4/6 exact post-spacing, got %d",exact)}
 negated:=thaiSpaceProjection("ไม่ ใช่ ใบ น ั ด ห ม า ย")
 // A negated statement still contains the positive title as a substring.
 // Therefore even successful normalization alone is NOT a safe classifier.
 falsePositive:=strings.Contains(negated,"ใบนัดหมาย")
 if !falsePositive {t.Fatal("negation hazard unexpectedly disappeared")}
 t.Logf("FOLIO_SYNTHETIC_THAI_SPACING_PROBE exact_positive=%d/6 negated_positive_keyword_match=%v auto_apply=false",exact,falsePositive)
}
