// Package pdfderivation defines a source-only typed contract for verified
// virtual-printer PDF outputs. This package performs no OCR, filesystem
// mutation, network calls, printer action, or privileged actor dispatch.
package pdfderivation

import (
    "encoding/hex"
    "errors"
    "fmt"
    "strings"
)

const (
    SourcePDF = "application/pdf"
    SourceURF = "image/urf"
    OutputPDF = "application/pdf"
)

type State string
const (
    Pending State = "pending"
    Ready State = "ready"
    NeedsReview State = "needs_review"
    Failed State = "failed"
)

type Editability string
const (
    // NativeEditable means original source PDF has retained native text
    // objects and an actual visible edit was independently verified.
    NativeEditable Editability = "native-editable"
    // ReconstructedEditable means the derived PDF has editable visible
    // text objects validated by a real editing oracle, not only OCR.
    ReconstructedEditable Editability = "reconstructed-editable"
    // SearchableImage means rendered pages include a searchable OCR layer,
    // which is NOT a promise of genuinely editable visible page content.
    SearchableImage Editability = "searchable-image"
)

type TextOrigin string
const (
    NativeText TextOrigin = "native"
    OCRText TextOrigin = "ocr"
    MixedText TextOrigin = "mixed"
)

// Evidence describes independent checks on OUTPUT bytes. A producer must not
// substitute its intended command options for positive result readback.
type Evidence struct {
    ParsedPDF bool
    PageParityVerified bool
    SearchableUnicodeVerified bool
    EmbeddedMetadataReadbackVerified bool
    VisibleTextEditVerified bool
    NativeTextPreserved bool
    ForcedRasterization bool
}

// Metadata is a proposed embedded Info/XMP summary. Optional fields such as
// Author must remain empty when not genuinely provided or verified.
type Metadata struct {
    Title string
    Author string
    Subject string
    Language string
    Creator string
    SourceTitleProvenance string
}

// Receipt is a typed, read-only projection of an independently committed
// immutable derived PDF linked to the immutable source print artifact.
type Receipt struct {
    SourceSHA256 string
    SourceMIME string
    OutputSHA256 string
    OutputMIME string
    Pages int
    ProcessorProfile string
    State State
    Editability Editability
    TextOrigin TextOrigin
    Metadata Metadata
    Evidence Evidence
    ReasonCode string
}

func digestValid(v string) bool {
    if len(v)!=64 || strings.ToLower(v)!=v {return false}
    b,err:=hex.DecodeString(v)
    return err==nil && len(b)==32
}

// ValidateReady never tries to infer editability from a producer's claim.
// Acceptance requires explicit independent proof of output searchability,
// embedding/metadata roundtrip, page parity, and for "editable" classes
// a completed visible-text edit rather than only invisible OCR overlay.
func (r Receipt) ValidateReady() error {
    if !digestValid(r.SourceSHA256) {return errors.New("invalid_source_digest")}
    if r.SourceMIME!=SourcePDF && r.SourceMIME!=SourceURF {
        return errors.New("unsupported_source_mime")
    }
    if r.State!=Ready {return errors.New("pdf_not_ready")}
    if !digestValid(r.OutputSHA256) || r.OutputMIME!=OutputPDF {
        return errors.New("missing_verified_pdf_output")
    }
    if r.ProcessorProfile=="" || r.Pages<1 {return errors.New("missing_derivation_identity")}
    if !r.Evidence.ParsedPDF || !r.Evidence.PageParityVerified ||
        !r.Evidence.SearchableUnicodeVerified ||
        !r.Evidence.EmbeddedMetadataReadbackVerified {
        return errors.New("pdf_result_oracle_incomplete")
    }
    switch r.Editability {
    case NativeEditable:
        if r.SourceMIME!=SourcePDF || r.TextOrigin==OCRText ||
            !r.Evidence.NativeTextPreserved ||
            r.Evidence.ForcedRasterization ||
            !r.Evidence.VisibleTextEditVerified {
            return errors.New("native_editability_not_proven")
        }
    case ReconstructedEditable:
        if !r.Evidence.VisibleTextEditVerified {
            return errors.New("reconstructed_editability_not_proven")
        }
    case SearchableImage:
        // OCR text can be selectable without editing the visible page.
        // No visible-editability claim is made here.
        if r.TextOrigin!=OCRText && r.TextOrigin!=MixedText {
            return errors.New("searchable_image_origin_invalid")
        }
    default:
        return fmt.Errorf("unrecognized_editability_tier: %q",r.Editability)
    }
    return nil
}
