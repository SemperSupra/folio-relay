package pdfderivation

import (
    "crypto/sha256"
    "encoding/hex"
    "strings"
    "testing"
)

func hash(b string) string {
    sum:=sha256.Sum256([]byte(b))
    return hex.EncodeToString(sum[:])
}

func fixture(sourceMIME string, e Editability) Receipt {
    return Receipt{
        SourceSHA256:hash("untouched source"),SourceMIME:sourceMIME,
        OutputSHA256:hash("verified output pdf"),OutputMIME:OutputPDF,
        Pages:2,ProcessorProfile:"ocrmypdf-test/1",State:Ready,
        Editability:e,TextOrigin:OCRText,
        Metadata:Metadata{Title:"Quarterly receipt",Creator:"FolioRelay source-only test"},
        Evidence:Evidence{
            ParsedPDF:true,PageParityVerified:true,
            SearchableUnicodeVerified:true,
            EmbeddedMetadataReadbackVerified:true,
        },
    }
}

func TestSearchableRasterPassesOnlyAsSearchableImage(t *testing.T) {
    r:=fixture(SourceURF,SearchableImage)
    if err:=r.ValidateReady();err!=nil{t.Fatal(err)}
    // OCR overlay never alone proves visible page text can be edited.
    r.Editability=ReconstructedEditable
    if err:=r.ValidateReady();err==nil || err.Error()!="reconstructed_editability_not_proven"{
        t.Fatalf("hidden OCR was mislabelled as editable: %v",err)
    }
    r.Evidence.VisibleTextEditVerified=true
    if err:=r.ValidateReady();err!=nil{t.Fatal(err)}
}

func TestNativePDFMustRetainRealTextAndNotFlatten(t *testing.T) {
    r:=fixture(SourcePDF,NativeEditable)
    r.TextOrigin=NativeText
    r.Evidence.NativeTextPreserved=true
    r.Evidence.VisibleTextEditVerified=true
    if err:=r.ValidateReady();err!=nil{t.Fatal(err)}
    r.Evidence.ForcedRasterization=true
    if err:=r.ValidateReady();err==nil{t.Fatal("rasterized native PDF was called editable")}
    r.Evidence.ForcedRasterization=false
    r.SourceMIME=SourceURF
    if err:=r.ValidateReady();err==nil{t.Fatal("raster URF was called native-editable")}
}

func TestReadyRequiresVerifiedTextPagesAndMetadata(t *testing.T) {
    baseline:=fixture(SourceURF,SearchableImage)
    cases:=[]struct{
        name string
        damage func(*Receipt)
    }{
        {"missing_text",func(r *Receipt){r.Evidence.SearchableUnicodeVerified=false}},
        {"metadata_no_readback",func(r *Receipt){r.Evidence.EmbeddedMetadataReadbackVerified=false}},
        {"broken_pdf",func(r *Receipt){r.Evidence.ParsedPDF=false}},
        {"page_mismatch",func(r *Receipt){r.Evidence.PageParityVerified=false}},
        {"missing_output",func(r *Receipt){r.OutputSHA256=""}},
        {"forged_digest_case",func(r *Receipt){r.OutputSHA256=strings.ToUpper(r.OutputSHA256)}},
        {"wrong_output_mime",func(r *Receipt){r.OutputMIME="image/urf"}},
        {"unsupported_source",func(r *Receipt){r.SourceMIME="image/jpeg"}},
        {"no_version",func(r *Receipt){r.ProcessorProfile=""}},
        {"no_pages",func(r *Receipt){r.Pages=0}},
        {"pending",func(r *Receipt){r.State=Pending}},
        {"review",func(r *Receipt){r.State=NeedsReview}},
        {"unknown_editability",func(r *Receipt){r.Editability="magic-editable"}},
        {"wrong_ocr_origin",func(r *Receipt){r.TextOrigin=NativeText}},
    }
    for _,tc:=range cases {
        t.Run(tc.name,func(t *testing.T) {
            r:=baseline
            tc.damage(&r)
            if err:=r.ValidateReady();err==nil {t.Fatal("unverified pdf unexpectedly ready")}
        })
    }
}

func TestMetadataUnknownAuthorNotInvented(t *testing.T) {
    r:=fixture(SourceURF,SearchableImage)
    if r.Metadata.Author!="" {t.Fatal("unknown author has been invented")}
    // Additional metadata should be evidence-backed and embed/readback
    // validated by the future OCR worker; this source contract does not
    // attempt to extract private records from arbitrary printed documents.
    if err:=r.ValidateReady();err!=nil{t.Fatal(err)}
}
