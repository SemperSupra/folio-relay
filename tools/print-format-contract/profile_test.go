package printformat

import (
    "os"
    "path/filepath"
    "sort"
    "strings"
    "testing"

    "github.com/SemperSupra/folio-relay/internal/airprint"
)

// Retained sources are identity-copied: no FolioRelay conversion engine.
var retainedFormats=[]string{"application/pdf","image/urf"}

func readRootFile(t *testing.T, parts ...string) string {
    t.Helper()
    p:=filepath.Join(append([]string{"..",".."},parts...)...)
    b,err:=os.ReadFile(p)
    if err!=nil{t.Fatal(err)}
    return string(b)
}

func TestAirPrintPPDAndBackendFormatsStayAligned(t *testing.T) {
    advertised:=strings.Split(airprint.PDL,",")
    sort.Strings(advertised)
    if strings.Join(advertised,",")!=strings.Join(retainedFormats,","){
        t.Fatalf("unqualified AirPrint MIME drift: %v",advertised)
    }

    ppd:=readRootFile(t,"experiments","print-substrate","cups-foliorelay.ppd")
    var formats []string
    const prefix="*cupsFilter2: "
    for _,raw:=range strings.Split(ppd,"\n"){
        line:=strings.TrimSpace(raw) // Cross-platform CRLF/ LF safe
        if !strings.HasPrefix(line,prefix){continue}
        spec:=strings.TrimPrefix(line,prefix)
        if !strings.HasPrefix(spec,"\"") || !strings.HasSuffix(spec,"\""){
            t.Fatalf("invalid PPD source format declaration %q",line)
        }
        f:=strings.Fields(strings.Trim(spec,"\""))
        if len(f)!=4 || f[2]!="0" || f[3]!="-" || f[0]!=f[1]{
            t.Fatalf("unqualified PPD converter/filter declaration: %q",line)
        }
        formats=append(formats,f[0])
    }
    sort.Strings(formats)
    if strings.Join(formats,",")!=strings.Join(retainedFormats,","){
        t.Fatalf("PPD identity formats %v do not match retained PDF/URF",formats)
    }

    backend:=readRootFile(t,"experiments","print-substrate","cups-backend.sh")
    if !strings.Contains(backend,"application/pdf|image/urf)") ||
       !strings.Contains(backend,"unsupported FolioRelay media type"){
        t.Fatal("CUPS final MIME whitelist changed without format qualification")
    }
    for _,bad:=range []string{"application/postscript|","image/jpeg|","image/png|"}{
        if strings.Contains(backend,bad){
            t.Fatalf("unexpected unqualified CUPS ingest format %q",bad)
        }
    }
}

func TestPrinterIsCaptureOnlyWithQualifiedPageSizes(t *testing.T) {
    conf:=readRootFile(t,"deploy","engineering-cups","printers.conf")
    if !strings.Contains(conf,"DeviceURI foliorelay:/"){
        t.Fatal("printer URI changed from capture-only backend")
    }
    if !strings.Contains(conf,"Type 4100"){
        t.Fatal("observed B&W and small-media printer type changed")
    }
    ppd:=readRootFile(t,"experiments","print-substrate","cups-foliorelay.ppd")
    for _,size:=range []string{"*PageSize A4/A4:","*PageSize Letter/US Letter:"}{
        if !strings.Contains(ppd,size){t.Fatalf("PPD page size missing %q",size)}
    }
}
