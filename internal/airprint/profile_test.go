package airprint

import (
    "os"
    "path/filepath"
    "regexp"
    "sort"
    "strings"
    "testing"
)

// These are the only currently qualified, unconverted artifact formats.
// This source assertion is not evidence that a later image was installed.
var retainedFormats = []string{"application/pdf", "image/urf"}

func readSource(t *testing.T, path ...string) string {
    t.Helper()
    all:=append([]string{"..",".."},path...)
    data,err:=os.ReadFile(filepath.Join(all...))
    if err!=nil{t.Fatal(err)}
    return string(data)
}

func TestRetainedPrintFormatAdvertisedParity(t *testing.T) {
    advertised:=strings.Split(PDL,",")
    sort.Strings(advertised)
    if strings.Join(advertised,",")!=strings.Join(retainedFormats,",") {
        t.Fatalf("AirPrint advertised unsupported or omitted formats: %v",advertised)
    }

    ppd:=readSource(t,"experiments","print-substrate","cups-foliorelay.ppd")
    line:=regexp.MustCompile(`(?m)^\*cupsFilter2: "([^ ]+) ([^ ]+) 0 -"$`)
    matches:=line.FindAllStringSubmatch(ppd,-1)
    if len(matches)!=len(retainedFormats) {
        t.Fatalf("PPD advertised %d filter paths, expected %d",len(matches),len(retainedFormats))
    }
    got:=make([]string,0,len(matches))
    for _,m:=range matches{
        if m[1]!=m[2] {
            t.Fatalf("printer now converts %s to %s; requires independent format qualification",m[1],m[2])
        }
        got=append(got,m[1])
    }
    sort.Strings(got)
    if strings.Join(got,",")!=strings.Join(retainedFormats,","){
        t.Fatalf("PPD formats %v do not match AirPrint/retained formats %v",got,retainedFormats)
    }

    backend:=readSource(t,"experiments","print-substrate","cups-backend.sh")
    // Pin the fail-closed final MIME switch at the actual print ingest
    // boundary, not only the client-visible PPD and DNS-SD string.
    if !strings.Contains(backend,"application/pdf|image/urf)") ||
        !strings.Contains(backend,"unsupported FolioRelay media type") {
        t.Fatal("CUPS ingest whitelist no longer matches PDF/URF; review before exposing new formats")
    }
    if strings.Contains(backend,"application/postscript|") ||
       strings.Contains(backend,"image/jpeg|") ||
       strings.Contains(backend,"image/png|") {
        t.Fatal("unexpected unqualified backend format")
    }
}

func TestPrintFormatSurfaceDoesNotPromisePhysicalOutput(t *testing.T) {
    printer:=readSource(t,"deploy","engineering-cups","printers.conf")
    if !strings.Contains(printer,"DeviceURI foliorelay:/"){
        t.Fatal("current printer no longer points exclusively to FolioRelay retention backend")
    }
    ppd:=readSource(t,"experiments","print-substrate","cups-foliorelay.ppd")
    for _,size:=range []string{"*PageSize A4/A4:","*PageSize Letter/US Letter:"} {
        if !strings.Contains(ppd,size){t.Fatalf("source page size absent: %q",size)}
    }
    // Type 4100 corresponds to a B&W small-format CUPS printer. RGB in a
    // PPD is not proof of a color-printing output route.
    if !strings.Contains(printer,"Type 4100"){
        t.Fatal("B&W/small-media IPP printer-type projection changed without acceptance")
    }
}
